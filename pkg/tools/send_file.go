package tools

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SendFileTool sends a file (image, audio, etc.) to the model for analysis.
// For images: sends as vision content block.
// For audio: transcribes via Whisper (if available) or sends as base64.
// For other files: reads content and sends as text.
type SendFileTool struct {
	allowedDirs []string
}

// NewSendFileTool creates a new send_file tool
func NewSendFileTool(allowedDirs []string) *SendFileTool {
	return &SendFileTool{allowedDirs: allowedDirs}
}

func (t *SendFileTool) Name() string {
	return "send_file"
}

func (t *SendFileTool) Description() string {
	return "Send a file (image, audio, document) to the model for analysis. Images are sent as vision content. Audio files are transcribed if possible. Returns analysis-ready content."
}

func (t *SendFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to send (image, audio, or document)",
			},
			"purpose": map[string]any{
				"type":        "string",
				"description": "What you want the model to do with this file: 'analyze' (default), 'describe', 'extract_text'",
				"enum":        []string{"analyze", "describe", "extract_text"},
				"default":     "analyze",
			},
		},
		"required": []string{"path"},
	}
}

func (t *SendFileTool) Execute(params map[string]string) ToolResult {
	path := params["path"]
	if path == "" {
		return ToolResult{Error: "path parameter is required"}
	}

	// Security check
	if !IsPathAllowed(path, t.allowedDirs) {
		return ToolResult{Error: fmt.Sprintf("access denied: path %s is not in allowed directories", path)}
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to read file: %s", err)}
	}

	if len(data) == 0 {
		return ToolResult{Error: "file is empty"}
	}

	ext := strings.ToLower(filepath.Ext(path))
	format := DetectImageFormat(path)

	// Check file type
	switch {
	case isImageFile(ext):
		// Image file — encode as base64 and return with metadata
		base64Data := base64.StdEncoding.EncodeToString(data)
		return ToolResult{
			Output: fmt.Sprintf("Image file: %s (%d bytes, %s format). Sent as vision content for analysis.", filepath.Base(path), len(data), format),
			Metadata: map[string]any{
				"image_base64": base64Data,
				"image_format": format,
				"file_path":    path,
				"file_size":    len(data),
			},
		}

	case isAudioFile(ext):
		// Audio file — for now, return info about the file
		// Future: integrate with Whisper API for transcription
		base64Data := base64.StdEncoding.EncodeToString(data)
		return ToolResult{
			Output: fmt.Sprintf("Audio file: %s (%d bytes, %s format). Audio content attached.", filepath.Base(path), len(data), ext),
			Metadata: map[string]any{
				"audio_base64":  base64Data,
				"audio_format":  ext,
				"file_path":     path,
				"file_size":     len(data),
			},
		}

	default:
		// Text file — just read and return content
		content := string(data)
		if len(content) > 50000 {
			content = content[:50000] + "\n... [truncated]"
		}
		return ToolResult{
			Output: fmt.Sprintf("File: %s (%d bytes)\n\n%s", filepath.Base(path), len(data), content),
		}
	}
}

// isImageFile checks if the file extension is an image format
func isImageFile(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff", ".tif", ".ico":
		return true
	default:
		return false
	}
}

// isAudioFile checks if the file extension is an audio format
func isAudioFile(ext string) bool {
	switch ext {
	case ".mp3", ".wav", ".ogg", ".flac", ".m4a", ".aac", ".wma", ".opus":
		return true
	default:
		return false
	}
}