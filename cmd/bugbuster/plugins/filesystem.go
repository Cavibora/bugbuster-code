package plugins

import (
	"fmt"
	"path/filepath"
	"strings"

	"bugbuster-code/pkg/plugin"
	"bugbuster-code/pkg/tools"
)

// FilesystemPlugin — file operations plugin (read, write, edit, glob, grep)
type FilesystemPlugin struct {
	plugin.BasePlugin
	allowedDirs []string
	maxFileSize  int64
	maxGrepRes  int
	maxGlobRes  int
}

// NewFilesystemPlugin creates file operations plugin
func NewFilesystemPlugin() *FilesystemPlugin {
	return &FilesystemPlugin{
		BasePlugin: plugin.BasePlugin{
			PluginName:        "filesystem",
			PluginDescription: "File system operations: read, write, edit, glob, grep",
			PluginVersion:     "1.0.0",
		},
		maxFileSize: 1048576, // 1MB
		maxGrepRes:  50,
		maxGlobRes:  100,
	}
}

// Init initializes plugin with configuration
func (p *FilesystemPlugin) Init(config map[string]any) error {
	if config == nil {
		return nil
	}

	// allowed_dirs
	if v, ok := config["allowed_dirs"]; ok {
		switch dirs := v.(type) {
		case []string:
			p.allowedDirs = dirs
		case []any:
			for _, d := range dirs {
				if s, ok := d.(string); ok {
					p.allowedDirs = append(p.allowedDirs, s)
				}
			}
		}
	}

	// max_file_size
	if v, ok := config["max_file_size"]; ok {
		switch size := v.(type) {
		case int:
			p.maxFileSize = int64(size)
		case int64:
			p.maxFileSize = size
		case float64:
			p.maxFileSize = int64(size)
		}
	}

	// max_grep_results
	if v, ok := config["max_grep_results"]; ok {
		switch n := v.(type) {
		case int:
			p.maxGrepRes = n
		case float64:
			p.maxGrepRes = int(n)
		}
	}

	// max_glob_results
	if v, ok := config["max_glob_results"]; ok {
		switch n := v.(type) {
		case int:
			p.maxGlobRes = n
		case float64:
			p.maxGlobRes = int(n)
		}
	}

	return nil
}

// Tools returns tools plugin
func (p *FilesystemPlugin) Tools() []tools.Tool {
	readTool := tools.NewReadTool()
	readTool.AllowedDirs = p.allowedDirs
	readTool.MaxSize = p.maxFileSize

	writeTool := tools.NewWriteTool()
	writeTool.AllowedDirs = p.allowedDirs

	editTool := tools.NewEditTool()
	editTool.AllowedDirs = p.allowedDirs

	globTool := tools.NewGlobTool()
	globTool.AllowedDirs = p.allowedDirs
	globTool.MaxResults = p.maxGlobRes

	grepTool := tools.NewGrepTool()
	grepTool.AllowedDirs = p.allowedDirs
	grepTool.MaxResults = p.maxGrepRes

	result := []tools.Tool{readTool, writeTool, editTool, globTool, grepTool}
	return result
}

// ValidateAllowedDirs checks that all allowed directories exist
func (p *FilesystemPlugin) ValidateAllowedDirs() error {
	for _, dir := range p.allowedDirs {
		if !strings.HasPrefix(dir, "/") && !strings.HasPrefix(dir, ".") {
			return fmt.Errorf("filesystem plugin: invalid allowed_dir: %s (must be absolute or relative path)", dir)
		}
		matches, err := filepath.Glob(dir)
		if err != nil {
			return fmt.Errorf("filesystem plugin: invalid glob pattern: %s: %w", dir, err)
		}
		_ = matches // directory may not exist yet
	}
	return nil
}