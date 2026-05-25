package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bugbuster-code/cmd/bugbuster/plugins"
	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/mcp"
	"bugbuster-code/pkg/plugin"
	"bugbuster-code/pkg/provider"
	"bugbuster-code/pkg/tools"

	"github.com/fatih/color"
)

// createProvider creates provider from configuration
func createProvider(cfg *config.BugBusterConfig) (provider.Provider, error) {
	providerName := cfg.DefaultProvider
	if model != "" {
		for name, prov := range cfg.Providers {
			if prov.Model == model {
				providerName = name
				break
			}
		}
	}

	provCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil, i18n.E("cli_error.provider_not_found", providerName, getProviderNames(cfg.Providers))
	}

	return provider.NewFromConfig(providerName, provCfg)
}

func getProviderNames(providers map[string]provider.ProviderConfig) []string {
	var names []string
	for name := range providers {
		names = append(names, name)
	}
	return names
}

// createAgentLoop creates and configures agent loop.
// If changeTracker != nil, write/edit tools are wrapped in HookedTool
// for automatic change tracking (undo).
func createAgentLoop(cfg *config.BugBusterConfig, p provider.Provider, changeTracker *ChangeTracker) *agent.AgentLoop {
	loop := agent.NewAgentLoop(p)

	loop.SetVerbose(verbose || cfg.Agent.Verbose)
	loop.SetDebug(debug)

	if debug {
		debugDir := filepath.Join(getProjectDir(cfg), ".bugbuster")
		os.MkdirAll(debugDir, 0755)
		loop.SetDebugDir(debugDir)
	}

	if cfg.Agent.MaxTokens > 0 {
		loop.SetMaxTokens(cfg.Agent.MaxTokens)
	}
	if cfg.Agent.KeepRecent > 0 {
		loop.SetKeepRecent(cfg.Agent.KeepRecent)
	}

	// LLM request timeouts
	if cfg.Agent.RequestTimeout > 0 {
		loop.SetRequestTimeout(time.Duration(cfg.Agent.RequestTimeout) * time.Second)
	}
	if cfg.Agent.ThinkingTimeout > 0 {
		loop.SetThinkingTimeout(time.Duration(cfg.Agent.ThinkingTimeout) * time.Second)
	}
	if cfg.Agent.IdleTimeout > 0 {
		loop.SetIdleTimeout(time.Duration(cfg.Agent.IdleTimeout) * time.Second)
	}

	// Loop detection settings
	if cfg.Agent.LoopDetection.RepeatThreshold > 0 {
		loop.SetLoopRepeatThreshold(cfg.Agent.LoopDetection.RepeatThreshold)
	}
	if cfg.Agent.LoopDetection.ToolRepeatThreshold > 0 {
		loop.SetLoopToolRepeatThreshold(cfg.Agent.LoopDetection.ToolRepeatThreshold)
	}
	if cfg.Agent.LoopDetection.WindowSize > 0 {
		loop.SetLoopWindowSize(cfg.Agent.LoopDetection.WindowSize)
	}
	if cfg.Agent.LoopDetection.TextSimilarityThreshold > 0 {
		loop.SetLoopTextSimilarityThreshold(cfg.Agent.LoopDetection.TextSimilarityThreshold)
	}
	if cfg.Agent.LoopDetection.TextSimilarityWindow > 0 {
		loop.SetLoopTextSimilarityWindow(cfg.Agent.LoopDetection.TextSimilarityWindow)
	}

	// Override MaxTokens from provider if context_window is specified
	provCfg := cfg.Providers[cfg.DefaultProvider]
	if provCfg.ContextWindow > 0 {
		loop.SetMaxTokens(provCfg.ContextWindow)
	}

	// LLM-based compaction — uses provider to generate summary
	loop.Context.Compactor = agent.NewLLMCompactor(p)

	if provCfg.Type == "cavibora" {
		loop.Context.SkipCompaction = true
	}

	// Create all tools
	readTool := tools.NewReadTool()
	readTool.AllowedDirs = cfg.Tools.AllowedDirs
	readTool.MaxSize = cfg.Tools.MaxFileSize

	writeTool := tools.NewWriteTool()
	writeTool.AllowedDirs = cfg.Tools.AllowedDirs

	editTool := tools.NewEditTool()
	editTool.AllowedDirs = cfg.Tools.AllowedDirs

	// Allow access to secret files depending on permission_mode
	// auto-approve and ask → allow (user trusts model or will confirm)
	// deny → block
	secretPermMode := agent.PermissionMode(cfg.Agent.PermissionMode)
	if permissionMode != "" {
		secretPermMode = agent.PermissionMode(permissionMode)
	}
	if secretPermMode == "" {
		secretPermMode = agent.PermissionAutoApprove
	}
	allowSecretFiles := secretPermMode == agent.PermissionAutoApprove || secretPermMode == agent.PermissionAsk
	readTool.AllowSecretFiles = allowSecretFiles
	writeTool.AllowSecretFiles = allowSecretFiles
	editTool.AllowSecretFiles = allowSecretFiles

	bashTool := tools.NewBashTool()
	bashTool.AllowedDirs = cfg.Tools.AllowedDirs
	effectiveSecurity := cfg.EffectiveSecurity(&provCfg)
	bashTool.BlockedCommands = effectiveSecurity.BlockedCommands
	bashTool.AllowNetwork = effectiveSecurity.AllowNetwork
	if cfg.Tools.BashTimeout > 0 {
		bashTool.Timeout = time.Duration(cfg.Tools.BashTimeout) * time.Second
	}
	if projectDir != "" {
		bashTool.DefaultDir = projectDir
	}

	grepTool := tools.NewGrepTool()
	grepTool.AllowedDirs = cfg.Tools.AllowedDirs
	grepTool.MaxResults = cfg.Tools.MaxGrepResults

	globTool := tools.NewGlobTool()
	globTool.AllowedDirs = cfg.Tools.AllowedDirs
	globTool.MaxResults = cfg.Tools.MaxGlobResults

	askTool := tools.NewAskTool()
	askTool.Provider = p

	askUserTool := tools.NewAskUserTool()

	webFetchTool := tools.NewWebFetchTool()
	webFetchTool.AllowNetwork = effectiveSecurity.AllowNetwork

	browseTool := tools.NewBrowseTool()
	browseTool.AllowNetwork = effectiveSecurity.AllowNetwork

	learnTool := tools.NewLearnTool()
	if provCfg.Type == "cavibora" {
		learnTool.TeachURL = provCfg.GetBaseURL()
		learnTool.APIKey = provCfg.APIKey
	}

	// Wrap write/edit in HookedTool for change tracking (undo)
	var writeWrapped tools.Tool = writeTool
	var editWrapped tools.Tool = editTool
	if changeTracker != nil {
		writeWrapped = tools.NewHookedTool(writeTool, &tools.ToolHook{
			Name: "undo_tracker_write",
			BeforeExecute: func(toolName string, params map[string]string) (map[string]string, error) {
				path := params["path"]
				if path != "" {
					changeTracker.RecordWrite(path, params["content"])
				}
				return params, nil
			},
		})

		editWrapped = tools.NewHookedTool(editTool, &tools.ToolHook{
			Name: "undo_tracker_edit",
			BeforeExecute: func(toolName string, params map[string]string) (map[string]string, error) {
				path := params["path"]
				if path != "" {
					data, err := os.ReadFile(path)
					if err == nil {
						oldContent := string(data)
						newContent := strings.Replace(oldContent, params["old"], params["new"], 1)
						changeTracker.RecordEdit(path, oldContent, newContent)
					}
				}
				return params, nil
			},
		})
	}

	// Register tools
	loop.RegisterTool(readTool)
	loop.RegisterTool(writeWrapped)
	loop.RegisterTool(editWrapped)
	loop.RegisterTool(bashTool)
	loop.RegisterTool(grepTool)
	loop.RegisterTool(globTool)
	loop.RegisterTool(askTool)
	loop.RegisterTool(askUserTool)
	loop.RegisterTool(webFetchTool)
	loop.RegisterTool(browseTool)
	loop.RegisterTool(learnTool)

	// Memory tool — persistent memory for important project facts
	memoryFilePath := filepath.Join(getProjectDir(cfg), ".bugbuster", "memory.md")
	loop.RegisterTool(tools.NewMemoryTool(memoryFilePath))

	// Todo-tools (checklist for planning)
	todoWrite := tools.NewTodoWriteTool()
	loop.RegisterTool(todoWrite)
	loop.RegisterTool(tools.NewTodoReadTool(todoWrite))

	// Subagents (task delegation)
	subagentConfig := agent.DefaultSubagentConfig()
	subagentConfig.Compactor = loop.Context.Compactor
	// Inherit request timeout from parent if larger than default
	if loop.RequestTimeout > 0 && loop.RequestTimeout > subagentConfig.Timeout {
		subagentConfig.Timeout = loop.RequestTimeout
	}
	loop.EnableSubagents(subagentConfig)

	// LSP-tool (code intelligence)
	lspTool := tools.NewLSPTool()
	lspTool.AllowedDirs = cfg.Tools.AllowedDirs
	if cfg.LSP.Timeout > 0 {
		lspTool.Timeout = time.Duration(cfg.LSP.Timeout) * time.Second
	}
	for lang, srv := range cfg.LSP.Servers {
		lspTool.Servers[lang] = tools.LSPServerConfig{
			Command: srv.Command,
			Args:    srv.Args,
		}
	}
	if projectDir != "" {
		lspTool.SetRootDir(projectDir)
	}
	loop.RegisterTool(lspTool)

	// System prompt
	dir := projectDir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	// Context archive — search through old messages
	if cfg.ContextArchive.Enabled {
		archiveDir := filepath.Join(getProjectDir(cfg), ".bugbuster", "context")
		archiveStore := agent.NewArchiveStore(archiveDir, cfg.ContextArchive.MaxBlocks)
		loop.Context.Archive = archiveStore
		loop.RegisterTool(agent.NewSearchContextTool(archiveStore))
		if cfg.ContextArchive.AutoOptimize {
			loop.Context.Optimizer = agent.NewArchiveOptimizer(archiveStore, loop.Context.Compactor)
		}
	}

	// Permissions
	permMode := agent.PermissionMode(cfg.Agent.PermissionMode)
	if permissionMode != "" {
		permMode = agent.PermissionMode(permissionMode)
	}
	if permMode == "" {
		permMode = agent.PermissionAutoApprove
	}
	permChecker := agent.NewDefaultPermissionChecker(permMode, dir)
	// In "ask" mode — connect interactive permission request
	if permMode == agent.PermissionAsk {
		permChecker.SetAskFunc(func(req agent.PermissionRequest) bool {
			fmt.Print(agent.FormatPermissionRequest(req))
			fmt.Print(i18n.T("agent.permission_approve") + " [y/N] ")
			var answer string
			fmt.Scanln(&answer)
			return strings.ToLower(answer) == "y" || strings.ToLower(answer) == "yes"
		})
	}
	loop.SetPermissionChecker(permChecker)

	// System prompt
	systemPrompt := agent.BuildSystemPrompt(dir, loop.Tools)

	// Inject memory into system prompt
	memoryContent := tools.LoadMemoryForPrompt(filepath.Join(dir, ".bugbuster", "memory.md"))
	if memoryContent != "" {
		systemPrompt += "\n\n" + memoryContent
	}

	loop.SetSystemPrompt(systemPrompt)

	// MCP-tools (from cfg.MCP.Servers and cfg.Plugins.MCP)
	mcpServers := cfg.MCP.Servers
	if cfg.Plugins.MCP != nil {
		if mcpServers == nil {
			mcpServers = make(map[string]config.MCPServerConfig)
		}
		for name, srv := range cfg.Plugins.MCP {
			mcpServers[name] = srv
		}
	}
	if mcpServers != nil {
		mgr := mcp.NewManager()
		mcpConfigs := make(map[string]mcp.ClientConfig)
		for name, srv := range mcpServers {
			mcpConfigs[name] = mcp.ClientConfig{
				Name:    name,
				Type:    srv.Type,
				Command: srv.Command,
				Args:    srv.Args,
				URL:     srv.URL,
				Env:     srv.Env,
				Headers: srv.Headers,
				Enabled: srv.Enabled,
			}
		}
		mgr.LoadFromConfig(mcpConfigs)
		if err := mgr.ConnectAll(context.Background()); err != nil {
			if verbose {
				color.Yellow("[MCP] %v", err)
			}
		}
		for _, tool := range mgr.GetAllTools() {
			loop.RegisterTool(tool)
		}
	}

	// Built-in plugins (builtins + enabled for backward compatibility)
	builtinNames := cfg.Plugins.Builtins
	if len(builtinNames) == 0 {
		builtinNames = cfg.Plugins.Enabled // backward compatibility
	}
	if len(builtinNames) > 0 {
		registry := plugin.NewRegistry()
		plugins.RegisterAll(registry)
		for _, pluginName := range builtinNames {
			pluginConfig := cfg.Plugins.Config[pluginName]
			_, err := registry.Load(pluginName, pluginConfig)
			if err != nil {
				if verbose {
					color.Yellow("[Plugin] %v", err)
				}
				continue
			}
			if verbose {
				color.Green("[Plugin] %s loaded", pluginName)
			}
		}
		registered := 0
		for _, tool := range registry.GetAllTools() {
			if _, exists := loop.Tools[tool.Name()]; !exists {
				loop.RegisterTool(tool)
				registered++
				if verbose {
					color.Green("[Plugin] +tool: %s", tool.Name())
				}
			} else if verbose {
				color.Yellow("[Plugin] tool %s skipped (built-in exists)", tool.Name())
			}
		}
		if verbose && registered > 0 {
			color.Green("[Plugin] Registered %d new tool(s) from plugins", registered)
		}
	}

	// External Go-plugins (.so)
	if len(cfg.Plugins.GoPlugins) > 0 {
		registry := plugin.NewRegistry()
		for _, gp := range cfg.Plugins.GoPlugins {
			_, err := registry.LoadSharedLibrary(gp.Path, gp.Config)
			if err != nil {
				color.Red("[Plugin] Failed to load %s: %v", gp.Name, err)
				continue
			}
			if verbose {
				color.Green("[Plugin] %s loaded from %s", gp.Name, gp.Path)
			}
		}
		for _, tool := range registry.GetAllTools() {
			if _, exists := loop.Tools[tool.Name()]; !exists {
				loop.RegisterTool(tool)
				if verbose {
					color.Green("[Plugin] +tool: %s", tool.Name())
				}
			} else if verbose {
				color.Yellow("[Plugin] tool %s skipped (already exists)", tool.Name())
			}
		}
	}

	return loop
}

// switchModel switches model
func switchModel(loop *agent.AgentLoop, cfg *config.BugBusterConfig, modelName string) {
	for name, prov := range cfg.Providers {
		if prov.Model == modelName {
			p, err := provider.NewFromConfig(name, prov)
			if err != nil {
				color.Red("%s", i18n.T("cli_error.model_switch", err))
				return
			}
			loop.SetProvider(p)
			color.Green("%s", i18n.T("cli_success.model_switched", modelName, name))
			return
		}
	}
	color.Red("%s", i18n.T("cli_error.model_not_found", modelName))
	for name, prov := range cfg.Providers {
		color.Yellow(i18n.T("cli_config.provider_entry", prov.Model, name))
	}
}

// switchProvider switches provider
func switchProvider(loop *agent.AgentLoop, cfg *config.BugBusterConfig, providerName string) {
	provCfg, ok := cfg.Providers[providerName]
	if !ok {
		color.Red("%s", i18n.T("cli_error.provider_not_found_short", providerName))
		for name := range cfg.Providers {
			color.Yellow("  - %s", name)
		}
		return
	}

	p, err := provider.NewFromConfig(providerName, provCfg)
	if err != nil {
		color.Red("%s", i18n.T("cli_error.provider_switch", err))
		return
	}

	loop.SetProvider(p)
	color.Green("%s", i18n.T("cli_success.provider_switched", providerName, provCfg.Model))
}

// getProjectDir returns project working directory
func getProjectDir(cfg *config.BugBusterConfig) string {
	if projectDir != "" {
		return projectDir
	}
	dir, _ := os.Getwd()
	return dir
}

// loadConfig loads configuration
func loadConfig() *config.BugBusterConfig {
	if cfgFile != "" {
		cfg, err := config.LoadConfig(cfgFile)
		if err != nil {
			color.Red("%s", i18n.T("cli_error.config_load", err))
			return config.DefaultConfig()
		}
		return cfg
	}

	cfgPath, err := config.FindConfigFile()
	if err != nil {
		return config.DefaultConfig()
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		color.Yellow("%s", i18n.T("cli_warning.config_default", err))
		return config.DefaultConfig()
	}

	return cfg
}

// printHelp prints help
func printHelp() {
	color.Cyan("%s", i18n.T("cli_help.title"))
	fmt.Println(i18n.T("cli_help.help"))
	fmt.Println(i18n.T("cli_help.exit"))
	fmt.Println(i18n.T("cli_help.reset"))
	fmt.Println(i18n.T("cli_help.context"))
	fmt.Println(i18n.T("cli_help.tools"))
	fmt.Println(i18n.T("cli_help.model"))
	fmt.Println(i18n.T("cli_help.provider"))
	fmt.Println(i18n.T("cli_help.sessions"))
	fmt.Println(i18n.T("cli_help.undo"))
	fmt.Println(i18n.T("cli_help.undo_all"))
	fmt.Println(i18n.T("cli_help.diff"))
	fmt.Println(i18n.T("cli_help.compact"))
	fmt.Println("  /plugin          — List, install, remove plugins")
	fmt.Println("  /plugin install <name> — Install plugin from registry")
	fmt.Println("  /plugin remove <name>  — Remove plugin from config")
	fmt.Println("  /debug — Toggle debug logging (shows raw SSE data)")
	fmt.Println("  /tui   — " + i18n.T("cli_help.tui"))
	fmt.Println("  /auto  — " + i18n.T("cli_help.auto"))
	fmt.Println("  /auto N — Autopilot with N iterations limit")
	fmt.Println("  /cli   — " + i18n.T("cli_help.cli"))
	fmt.Println()
	color.Cyan("%s", "🧠 Cavibora Commands")
	fmt.Println("  /dream [seed]  — Trigger memory consolidation")
	fmt.Println("  /emotions       — Show model emotional state")
	fmt.Println("  /mesh-stats     — Show associative mesh statistics")
	fmt.Println()
	color.Cyan("%s", i18n.T("cli.subcommands_header"))
	fmt.Println("  bugbuster scan [path]       — " + i18n.T("cli_subcommands.scan_short"))
	fmt.Println("  bugbuster fix [description] — " + i18n.T("cli_subcommands.fix_short"))
	fmt.Println("  bugbuster test [path]       — " + i18n.T("cli_subcommands.test_short"))
	fmt.Println("  bugbuster config show       — " + i18n.T("cli_subcommands.config_show_short"))
	fmt.Println("  bugbuster config init       — " + i18n.T("cli_subcommands.config_init_short"))
	fmt.Println("  bugbuster config providers  — " + i18n.T("cli_subcommands.config_providers_short"))
	fmt.Println("  bugbuster version           — " + i18n.T("cli_subcommands.version_short"))
}

// printTools prints tool list
func printTools(loop *agent.AgentLoop) {
	color.Cyan("%s", i18n.T("cli_help.available_tools"))
	for name, tool := range loop.Tools {
		fmt.Printf("  %-10s %s\n", name, tool.Description())
	}
}
