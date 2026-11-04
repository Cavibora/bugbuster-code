package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"bugbuster-code/pkg/agent"
	"bugbuster-code/pkg/config"
	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/serve"
	"bugbuster-code/pkg/theme"
	"bugbuster-code/pkg/tools"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	serveAddr           string
	servePort           int
	serveToken          string
	serveTokenFile      string
	serveNoToken        bool
	serveMaxSessions    int
	serveIdleMinutes    int
	servePersist        bool
	serveSessionsDir    string
	serveToolResultKB   int
	serveProvider       string
	servePrintTokenOnly bool
	serveAutoContinue   bool
)

// serveCmd creates the subcommand `serve` — an HTTP control plane for agents.
func serveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run BugBuster as an HTTP API server (sessions, tasks, SSE events)",
		Long: strings.TrimSpace(`
Start an HTTP server that exposes the agent to external processes: CI jobs,
chat bots, IDE plugins or another agent can create sessions, post a task,
stream the run as Server-Sent Events, inject clarifications mid-run and
cancel it.

Endpoints (all require "Authorization: Bearer <token>" unless --allow-no-token):
  GET    /health                          liveness, no auth
  GET    /v1/sessions                     list served sessions
  POST   /v1/sessions                     create session                 {name, task}
  GET    /v1/sessions/{id}                session state
  DELETE /v1/sessions/{id}                close session
  POST   /v1/sessions/{id}/task           start a task                   {task}
  POST   /v1/sessions/{id}/inject         message a running agent        {text}
  POST   /v1/sessions/{id}/answer         answer an ask_user question    {text}
  POST   /v1/sessions/{id}/cancel         stop the current run
  POST   /v1/sessions/{id}/compact        force compaction
  GET    /v1/sessions/{id}/events         SSE stream (?since=N&text_only=1&follow=1)
  GET    /v1/sessions/{id}/transcript     conversation so far
  POST   /v1/run                          one-shot task                  {task, wait}

Security: this API runs shell commands with your credentials. The server binds
to 127.0.0.1 by default and refuses to start without a token on a non-loopback
address. Put it behind an SSH tunnel or a local TLS proxy for remote access.

Examples:
  bugbuster serve --port 8765
  TOKEN=$(bugbuster serve --print-token)
  curl -H "Authorization: Bearer $TOKEN" -d '{"task":"summarize this repo"}' localhost:8765/v1/run
  curl -N -H "Authorization: Bearer $TOKEN" localhost:8765/v1/sessions/ID/events?text_only=1
`),
		Args: cobra.NoArgs,
		Run:  runServe,
	}

	cmd.Flags().StringVar(&serveAddr, "addr", "", "listen address (default from config or 127.0.0.1:8765)")
	cmd.Flags().IntVar(&servePort, "port", 0, "shorthand for --addr 127.0.0.1:<port>")
	cmd.Flags().StringVar(&serveToken, "token", "", "bearer token required by all endpoints")
	cmd.Flags().StringVar(&serveTokenFile, "token-file", "", "read the bearer token from a file")
	cmd.Flags().BoolVar(&serveNoToken, "allow-no-token", false, "run without auth (loopback addresses only)")
	cmd.Flags().IntVar(&serveMaxSessions, "max-sessions", 0, "max concurrent sessions (default 8)")
	cmd.Flags().IntVar(&serveIdleMinutes, "idle-minutes", -1, "close sessions idle longer than this, 0 = never (default 60)")
	cmd.Flags().BoolVar(&servePersist, "persist", true, "save session transcripts to .bugbuster/sessions")
	cmd.Flags().StringVar(&serveSessionsDir, "sessions-dir", "", "override the transcript directory")
	cmd.Flags().IntVar(&serveToolResultKB, "max-tool-result-kb", 0, "truncate tool payloads on the wire, KB (default 4)")
	cmd.Flags().StringVar(&serveProvider, "provider", "", "override default_provider for served sessions")
	cmd.Flags().BoolVar(&serveAutoContinue, "auto-continue", false, "keep the agent autopilot running after a text-only reply (default off: the API client decides when to continue)")
	cmd.Flags().BoolVar(&servePrintTokenOnly, "print-token", false, "generate a random token, print it and exit")

	return cmd
}

// runServe starts the HTTP control plane.
func runServe(cmd *cobra.Command, args []string) {
	if servePrintTokenOnly {
		tok, err := generateServeToken()
		if err != nil {
			color.Red("failed to generate token: %v", err)
			os.Exit(1)
		}
		fmt.Println(tok)
		return
	}

	cfg := loadConfig()

	// --provider overrides default_provider for every session this server
	// creates, so one deployment can point at a different model than the CLI.
	if serveProvider != "" {
		if _, ok := cfg.Providers[serveProvider]; !ok {
			color.Red("%s", i18n.E("cli_error.provider_not_found", serveProvider, getProviderNames(cfg.Providers)))
			os.Exit(1)
		}
		cfg.DefaultProvider = serveProvider
	}

	lang := langFlag
	if lang == "" {
		lang = cfg.Agent.Language
	}
	if lang == "" {
		lang = "en"
	}
	i18n.Init(lang)
	appTheme = theme.ResolveTheme(cfg.Theme)

	addr, err := resolveServeAddr(cmd, cfg)
	if err != nil {
		color.Red("%s", err)
		os.Exit(1)
	}

	token, tokenGenerated, err := resolveServeToken(cmd, cfg)
	if err != nil {
		color.Red("%s", err)
		os.Exit(1)
	}
	if token == "" && !serveNoToken {
		// Nothing configured: mint a one-time token rather than refusing to
		// start or silently running open. It is printed once below.
		token, err = generateServeToken()
		if err != nil {
			color.Red("failed to generate token: %v", err)
			os.Exit(1)
		}
		tokenGenerated = true
	}

	sessionsDir, err := resolveServeSessionsDir(cmd, cfg)
	if err != nil {
		color.Red("%s", err)
		os.Exit(1)
	}

	maxSessions := flagOrInt(cmd, "max-sessions", cfg.Serve.MaxSessions, 8)
	idleMinutes := flagOrInt(cmd, "idle-minutes", cfg.Serve.IdleMinutes, 60)
	toolResultKB := flagOrInt(cmd, "max-tool-result-kb", cfg.Serve.MaxToolResultKB, 4)
	autoContinue := serveAutoContinueValue(cmd, cfg)

	srv, err := serve.New(serve.Config{
		Addr:               addr,
		Token:              token,
		AllowNoToken:       serveNoToken,
		MaxSessions:        maxSessions,
		IdleTTL:            time.Duration(idleMinutes) * time.Minute,
		MaxToolResultBytes: toolResultKB * 1024,
		SessionsDir:        sessionsDir,
		Version:            Version,
		Factory:            serveLoopFactory(cfg, autoContinue),
	})
	if err != nil {
		color.Red("%s", i18n.T("cli_error.general", err))
		os.Exit(1)
	}

	printServeBanner(srv, cfg, token, tokenGenerated, sessionsDir, maxSessions, idleMinutes)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		color.Yellow("\n[serve] shutting down…")
		cancel()
	}()

	if err := srv.Serve(ctx); err != nil && !errorsIsContextCanceled(err) {
		color.Red("[serve] %v", err)
		os.Exit(1)
	}
}

// errorsIsContextCanceled reports whether err is the ctx cancellation sentinel.
func errorsIsContextCanceled(err error) bool {
	return err != nil && strings.Contains(err.Error(), "context canceled")
}

// resolveServeAddr builds the listen address from flags or config.
func resolveServeAddr(cmd *cobra.Command, cfg *config.BugBusterConfig) (string, error) {
	switch {
	case serveAddr != "" && servePort > 0:
		return "", fmt.Errorf("use either --addr or --port, not both")
	case serveAddr != "":
		return serveAddr, nil
	case servePort > 0:
		return fmt.Sprintf("127.0.0.1:%d", servePort), nil
	case cfg.Serve.Addr != "":
		return cfg.Serve.Addr, nil
	default:
		return "127.0.0.1:8765", nil
	}
}

// resolveServeToken picks the bearer token: flag > token-file > env > config.
// The second return value reports whether the token came from an explicit
// source (a generated one is safe to print, a configured one is not).
func resolveServeToken(cmd *cobra.Command, cfg *config.BugBusterConfig) (string, bool, error) {
	if serveToken != "" {
		return serveToken, true, nil
	}
	if serveTokenFile != "" {
		data, err := os.ReadFile(serveTokenFile)
		if err != nil {
			return "", false, fmt.Errorf("cannot read --token-file: %w", err)
		}
		return strings.TrimSpace(string(data)), true, nil
	}
	if env := os.Getenv("BUGBUSTER_SERVE_TOKEN"); env != "" {
		return env, true, nil
	}
	if cfg.Serve.Token != "" {
		return cfg.Serve.Token, true, nil
	}
	return "", false, nil
}

// resolveServeSessionsDir decides where transcripts live ("" = disabled).
func resolveServeSessionsDir(cmd *cobra.Command, cfg *config.BugBusterConfig) (string, error) {
	persist := servePersist
	if cfg.Serve.Persist != nil && !cmd.Flags().Changed("persist") {
		persist = *cfg.Serve.Persist
	}
	if !persist {
		return "", nil
	}
	dir := serveSessionsDir
	if dir == "" {
		dir = cfg.Serve.SessionsDir
	}
	if dir == "" {
		dir = filepath.Join(getProjectDir(cfg), ".bugbuster", "sessions")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("cannot create sessions dir %s: %w", dir, err)
	}
	return dir, nil
}

// flagOrInt resolves an int setting: explicit flag > config > default.
func flagOrInt(cmd *cobra.Command, flag string, fromCfg, def int) int {
	if cmd.Flags().Changed(flag) {
		return currentFlagValue(cmd, flag)
	}
	if fromCfg > 0 {
		return fromCfg
	}
	return def
}

// currentFlagValue reads the raw flag value for the known int flags.
func currentFlagValue(cmd *cobra.Command, flag string) int {
	switch flag {
	case "max-sessions":
		return serveMaxSessions
	case "idle-minutes":
		return serveIdleMinutes
	case "max-tool-result-kb":
		return serveToolResultKB
	}
	return 0
}

// serveAutoContinueValue resolves the autopilot setting: explicit flag > config
// > off. Serving is headless, so the client owns the "keep going?" decision;
// the TUI default (on) would otherwise make a one-word reply burn three
// iterations and minutes of model time.
func serveAutoContinueValue(cmd *cobra.Command, cfg *config.BugBusterConfig) bool {
	if cmd.Flags().Changed("auto-continue") {
		return serveAutoContinue
	}
	if cfg.Serve.AutoContinue != nil {
		return *cfg.Serve.AutoContinue
	}
	return false
}

// serveLoopFactory returns a function that builds a fully wired agent loop per
// session. It reuses createAgentLoop so tools, timeouts, fallbacks, hub and
// skills behave exactly as in the CLI. The extra steps below cover what the CLI
// normally does in its interactive shell: session identity, and the two places
// where a prompt would block on a terminal that does not exist here.
func serveLoopFactory(cfg *config.BugBusterConfig, autoContinue bool) serve.LoopFactory {
	return func(sessionID, name string) (*agent.AgentLoop, error) {
		p, err := createProvider(cfg)
		if err != nil {
			return nil, err
		}

		loop := createAgentLoop(cfg, p, nil, sessionID)
		loop.SetNonInteractive(true)
		loop.SetAutoContinue(autoContinue)

		// ask_user is bridged to the API by pkg/serve (POST .../answer), so the
		// tool must wait for a real answer instead of auto-skipping. The bridge
		// times out on its own, so a client that never answers cannot wedge the
		// run forever.
		if askUser, ok := loop.Tools["ask_user"].(*tools.AskUserTool); ok {
			askUser.NonInteractive = false
		}

		// Permission prompts cannot be read from stdin in server mode. Dropping
		// the AskFunc makes mode "ask" resolve to the safe default: approve
		// workspace writes, deny full-access actions.
		if pc, ok := loop.PermissionChecker.(*agent.DefaultPermissionChecker); ok {
			pc.SetAskFunc(nil)
		}

		// Stable provider-side session identity (x-opencode-session and friends).
		loop.SetSessionIDForProvider(sessionID)

		return loop, nil
	}
}

// printServeBanner shows how to reach the server and whether it is authenticated.
func printServeBanner(srv *serve.Server, cfg *config.BugBusterConfig, token string, tokenFromConfig bool, sessionsDir string, maxSessions, idleMinutes int) {
	color.Cyan("BugBuster Code %s — agent API server", Version)
	color.Green("  listening    http://%s", srv.Addr())

	provCfg := cfg.Providers[cfg.DefaultProvider]
	color.Cyan("  provider     %s (%s)", cfg.DefaultProvider, provCfg.Model)
	color.Cyan("  project      %s", getProjectDir(cfg))
	color.Cyan("  max sessions %d, idle eviction %s", maxSessions, idleLabel(idleMinutes))
	if sessionsDir != "" {
		color.Cyan("  transcripts  %s", sessionsDir)
	} else {
		color.Cyan("  transcripts  disabled (--persist=false)")
	}

	switch {
	case token == "":
		color.Yellow("  auth         DISABLED — any local process can run commands")
	case tokenFromConfig:
		color.Yellow("  auth         bearer token (from flags/env/config, not printed)")
	default:
		color.Yellow("  auth         bearer token (generated, shown once — save it now):")
		color.Cyan("               %s", token)
	}

	scheme := "http"
	fmt.Fprintf(os.Stderr, "\n")
	if token != "" {
		color.Cyan("  curl -H \"Authorization: Bearer $TOKEN\" %s://%s/health\n", scheme, srv.Addr())
	} else {
		color.Cyan("  curl %s://%s/health\n", scheme, srv.Addr())
	}
	color.Yellow("  Ctrl+C to stop\n")
}

// idleLabel renders the eviction setting for humans.
func idleLabel(minutes int) string {
	if minutes <= 0 {
		return "off"
	}
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%.1fh", float64(minutes)/60)
}

// generateServeToken returns a 32-hex-char random token.
func generateServeToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
