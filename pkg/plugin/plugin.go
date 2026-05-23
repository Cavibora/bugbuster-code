// Package plugin provides a plugin system for BugBuster Code.
// Plugins can register custom tools, hooks, and extend the agent's capabilities.
package plugin

import (
	"bugbuster-code/pkg/tools"
	"fmt"
	"plugin"
)

// Plugin — interface plugin
type Plugin interface {
	// Name returns unique plugin name
	Name() string

	// Description returns description plugin
	Description() string

	// Version returns plugin version
	Version() string

	// Init initializes plugin with configuration
	Init(config map[string]any) error

	// Tools returns tools provided by the plugin
	Tools() []tools.Tool

	// Shutdown is called on termination (for resource cleanup)
	Shutdown() error
}

// BasePlugin is base plugin implementation (embedding for convenience)
type BasePlugin struct {
	PluginName        string
	PluginDescription string
	PluginVersion     string
}

func (p *BasePlugin) Name() string                { return p.PluginName }
func (p *BasePlugin) Description() string         { return p.PluginDescription }
func (p *BasePlugin) Version() string             { return p.PluginVersion }
func (p *BasePlugin) Init(_ map[string]any) error { return nil }
func (p *BasePlugin) Tools() []tools.Tool         { return nil }
func (p *BasePlugin) Shutdown() error             { return nil }

// Registry — plugin registry
type Registry struct {
	plugins map[string]Plugin
	factory map[string]func() Plugin // factories for creating plugins by name
}

// NewRegistry creates a new plugin registry
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
		factory: make(map[string]func() Plugin),
	}
}

// Register registers a plugin in the registry
func (r *Registry) Register(p Plugin) error {
	if p == nil {
		return fmt.Errorf("plugin is nil")
	}
	name := p.Name()
	if name == "" {
		return fmt.Errorf("plugin name is empty")
	}
	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin '%s' already registered", name)
	}
	r.plugins[name] = p
	return nil
}

// RegisterFactory registers a factory for creating plugins
func (r *Registry) RegisterFactory(name string, factory func() Plugin) {
	r.factory[name] = factory
}

// Load loads plugin by name via factory and initializes it
func (r *Registry) Load(name string, config map[string]any) (Plugin, error) {
	factory, ok := r.factory[name]
	if !ok {
		return nil, fmt.Errorf("plugin factory '%s' not found", name)
	}

	p := factory()
	if err := p.Init(config); err != nil {
		return nil, fmt.Errorf("plugin '%s' init error: %w", name, err)
	}

	if err := r.Register(p); err != nil {
		return nil, err
	}

	return p, nil
}

// Get returns plugin by name
func (r *Registry) Get(name string) (Plugin, bool) {
	p, ok := r.plugins[name]
	return p, ok
}

// GetAllTools returns tools from all registered plugins
func (r *Registry) GetAllTools() []tools.Tool {
	var allTools []tools.Tool
	for _, p := range r.plugins {
		allTools = append(allTools, p.Tools()...)
	}
	return allTools
}

// List returns list of registered plugins
func (r *Registry) List() []PluginInfo {
	var result []PluginInfo
	for _, p := range r.plugins {
		result = append(result, PluginInfo{
			Name:        p.Name(),
			Description: p.Description(),
			Version:     p.Version(),
			ToolCount:   len(p.Tools()),
		})
	}
	return result
}

// ShutdownAll calls Shutdown() for all plugins
func (r *Registry) ShutdownAll() []error {
	var errs []error
	for _, p := range r.plugins {
		if err := p.Shutdown(); err != nil {
			errs = append(errs, fmt.Errorf("plugin '%s' shutdown error: %w", p.Name(), err))
		}
	}
	return errs
}

// PluginInfo is information about a plugin
type PluginInfo struct {
	Name        string
	Description string
	Version     string
	ToolCount   int
	Source      string // "builtin", "so", "mcp"
}

// LoadSharedLibrary loads Go-plugin from .so file
// .so file must export Plugin type plugin.Plugin
func (r *Registry) LoadSharedLibrary(path string, config map[string]any) (Plugin, error) {
	plug, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	sym, err := plug.Lookup("Plugin")
	if err != nil {
		return nil, fmt.Errorf("plugin %s: symbol 'Plugin' not found: %w", path, err)
	}

	p, ok := sym.(Plugin)
	if !ok {
		return nil, fmt.Errorf("plugin %s: symbol 'Plugin' is not a plugin.Plugin (got %T)", path, sym)
	}

	if err := p.Init(config); err != nil {
		return nil, fmt.Errorf("plugin %s init error: %w", path, err)
	}

	if err := r.Register(p); err != nil {
		return nil, err
	}

	return p, nil
}
