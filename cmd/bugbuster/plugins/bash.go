package plugins

import (
	"fmt"
	"time"

	"bugbuster-code/pkg/plugin"
	"bugbuster-code/pkg/tools"
)

// BashPlugin — bash command execution plugin
type BashPlugin struct {
	plugin.BasePlugin
	timeout       time.Duration
	blockedCmds  []string
	allowNetwork bool
	defaultDir   string
}

// NewBashPlugin creates plugin bash
func NewBashPlugin() *BashPlugin {
	return &BashPlugin{
		BasePlugin: plugin.BasePlugin{
			PluginName:        "bash",
			PluginDescription: "Execute bash commands with configurable permissions",
			PluginVersion:     "1.0.0",
		},
		timeout: 30 * time.Second,
	}
}

// Init initializes plugin with configuration
func (p *BashPlugin) Init(config map[string]any) error {
	if config == nil {
		return nil
	}

	// timeout (in seconds)
	if v, ok := config["timeout"]; ok {
		switch t := v.(type) {
		case int:
			p.timeout = time.Duration(t) * time.Second
		case float64:
			p.timeout = time.Duration(t) * time.Second
		case string:
			d, err := time.ParseDuration(t)
			if err == nil {
				p.timeout = d
			}
		}
	}

	// blocked_commands
	if v, ok := config["blocked_commands"]; ok {
		switch cmds := v.(type) {
		case []string:
			p.blockedCmds = cmds
		case []any:
			for _, c := range cmds {
				if s, ok := c.(string); ok {
					p.blockedCmds = append(p.blockedCmds, s)
				}
			}
		}
	}

	// allow_network
	if v, ok := config["allow_network"]; ok {
		if b, ok := v.(bool); ok {
			p.allowNetwork = b
		}
	}

	// default_dir
	if v, ok := config["default_dir"]; ok {
		if s, ok := v.(string); ok {
			p.defaultDir = s
		}
	}

	return nil
}

// Tools returns tools plugin
func (p *BashPlugin) Tools() []tools.Tool {
	bashTool := tools.NewBashTool()
	bashTool.Timeout = p.timeout
	bashTool.BlockedCommands = p.blockedCmds
	bashTool.AllowNetwork = p.allowNetwork
	bashTool.DefaultDir = p.defaultDir
	return []tools.Tool{bashTool}
}

// ValidateTimeout checks that timeout is within reasonable limits
func (p *BashPlugin) ValidateTimeout() error {
	if p.timeout < time.Second {
		return fmt.Errorf("bash plugin: timeout too small: %s (minimum 1s)", p.timeout)
	}
	if p.timeout > 5*time.Minute {
		return fmt.Errorf("bash plugin: timeout too large: %s (maximum 5m)", p.timeout)
	}
	return nil
}