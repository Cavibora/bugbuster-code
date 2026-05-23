package plugins

import (
	"time"

	"bugbuster-code/pkg/plugin"
	"bugbuster-code/pkg/tools"
)

// WebPlugin — web request plugin (web_fetch)
type WebPlugin struct {
	plugin.BasePlugin
	allowNetwork  bool
	timeout       time.Duration
	maxBodySize   int64
}

// NewWebPlugin creates web request plugin
func NewWebPlugin() *WebPlugin {
	return &WebPlugin{
		BasePlugin: plugin.BasePlugin{
			PluginName:        "web",
			PluginDescription: "Web fetch and search capabilities",
			PluginVersion:     "1.0.0",
		},
		allowNetwork: true,
		timeout:      30 * time.Second,
		maxBodySize:  1024 * 1024, // 1MB
	}
}

// Init initializes plugin with configuration
func (p *WebPlugin) Init(config map[string]any) error {
	if config == nil {
		return nil
	}

	// allow_network
	if v, ok := config["allow_network"]; ok {
		if b, ok := v.(bool); ok {
			p.allowNetwork = b
		}
	}

	// timeout
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

	// max_body_size
	if v, ok := config["max_body_size"]; ok {
		switch size := v.(type) {
		case int:
			p.maxBodySize = int64(size)
		case int64:
			p.maxBodySize = size
		case float64:
			p.maxBodySize = int64(size)
		}
	}

	return nil
}

// Tools returns tools plugin
func (p *WebPlugin) Tools() []tools.Tool {
	webFetchTool := tools.NewWebFetchTool()
	webFetchTool.AllowNetwork = p.allowNetwork
	return []tools.Tool{webFetchTool}
}