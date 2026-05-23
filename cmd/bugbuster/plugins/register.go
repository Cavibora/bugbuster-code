package plugins

import (
	"bugbuster-code/pkg/plugin"
)

// RegisterAll registers all plugin factories in registry
func RegisterAll(registry *plugin.Registry) {
	registry.RegisterFactory("filesystem", func() plugin.Plugin { return NewFilesystemPlugin() })
	registry.RegisterFactory("bash", func() plugin.Plugin { return NewBashPlugin() })
	registry.RegisterFactory("web", func() plugin.Plugin { return NewWebPlugin() })
}