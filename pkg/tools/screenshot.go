package tools

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ScreenshotTool captures screenshots (full screen, window, or region)
type ScreenshotTool struct{}

// NewScreenshotTool creates a new screenshot tool
func NewScreenshotTool() *ScreenshotTool {
	return &ScreenshotTool{}
}

func (t *ScreenshotTool) Name() string {
	return "screenshot"
}

func (t *ScreenshotTool) Description() string {
	return "Take a screenshot of the desktop, a specific window, or a region. Returns base64-encoded image for vision models."
}

func (t *ScreenshotTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"type":        "string",
				"description": "Screenshot mode: 'fullscreen' (entire screen), 'window' (click to select window), 'region' (specify x,y,width,height)",
				"enum":        []string{"fullscreen", "window", "region"},
				"default":     "fullscreen",
			},
			"region": map[string]any{
				"type":        "string",
				"description": "Region coordinates as 'x,y,width,height' (only for mode='region'). Example: '100,100,800,600'",
			},
			"format": map[string]any{
				"type":        "string",
				"description": "Image format: 'png' (default, lossless) or 'jpeg' (smaller size)",
				"enum":        []string{"png", "jpeg"},
				"default":     "png",
			},
		},
		"required": []string{},
	}
}

func (t *ScreenshotTool) Execute(params map[string]string) ToolResult {
	mode := params["mode"]
	if mode == "" {
		mode = "fullscreen"
	}

	format := params["format"]
	if format == "" {
		format = "png"
	}

	// Check if screencapture is available (macOS)
	screencapturePath, err := exec.LookPath("screencapture")
	if err != nil {
		// Try Linux alternatives
		if _, err := exec.LookPath("gnome-screenshot"); err == nil {
			return t.screenshotLinux(params, mode, format)
		}
		if _, err := exec.LookPath("scrot"); err == nil {
			return t.screenshotLinux(params, mode, format)
		}
		if _, err := exec.LookPath("import"); err == nil {
			return t.screenshotLinux(params, mode, format)
		}
		return ToolResult{Error: "screenshot not available: no screenshot tool found (screencapture, gnome-screenshot, scrot, or import required)"}
	}

	_ = screencapturePath // Use macOS screencapture

	// Build screencapture command
	tmpFile := filepath.Join("/tmp", fmt.Sprintf("bugbuster_screenshot.%s", format))

	var cmd *exec.Cmd
	switch mode {
	case "fullscreen":
		cmd = exec.Command("screencapture", "-x", tmpFile)
	case "window":
		// -w allows user to click a window, -x no sound
		cmd = exec.Command("screencapture", "-x", "-w", tmpFile)
	case "region":
		region := params["region"]
		if region == "" {
			return ToolResult{Error: "region parameter required for mode='region'. Format: 'x,y,width,height'"}
		}
		// -R x,y,w,h for region capture
		cmd = exec.Command("screencapture", "-x", "-R", region, tmpFile)
	default:
		return ToolResult{Error: fmt.Sprintf("unknown mode '%s'. Use: fullscreen, window, or region", mode)}
	}

	// Execute screencapture
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("screenshot failed: %s: %s", err, string(output))}
	}

	// Read the captured file
	data, err := exec.Command("cat", tmpFile).Output()
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to read screenshot: %s", err)}
	}

	// Clean up temp file
	_ = exec.Command("rm", tmpFile).Run()

	if len(data) == 0 {
		return ToolResult{Error: "screenshot captured but file is empty"}
	}

	// Encode to base64
	base64Data := base64.StdEncoding.EncodeToString(data)

	// Return as structured result with image data
	// The agent will convert this to an image content block
	return ToolResult{
		Output: fmt.Sprintf("Screenshot captured (%s, %d bytes, mode=%s).\nbase64 data follows:\n%s", format, len(data), mode, base64Data),
		Metadata: map[string]any{
			"image_base64": base64Data,
			"image_format": format,
			"image_size":   len(data),
			"mode":         mode,
		},
	}
}

func (t *ScreenshotTool) screenshotLinux(params map[string]string, mode, format string) ToolResult {
	tmpFile := filepath.Join("/tmp", fmt.Sprintf("bugbuster_screenshot.%s", format))

	var cmd *exec.Cmd

	// Try gnome-screenshot first
	if _, err := exec.LookPath("gnome-screenshot"); err == nil {
		switch mode {
		case "fullscreen":
			cmd = exec.Command("gnome-screenshot", "-f", tmpFile)
		case "window":
			cmd = exec.Command("gnome-screenshot", "-w", "-f", tmpFile)
		case "region":
			cmd = exec.Command("gnome-screenshot", "-a", "-f", tmpFile)
		}
	} else if _, err := exec.LookPath("scrot"); err == nil {
		switch mode {
		case "fullscreen":
			cmd = exec.Command("scrot", tmpFile)
		case "window":
			cmd = exec.Command("scrot", "-u", tmpFile)
		case "region":
			cmd = exec.Command("scrot", "-s", tmpFile)
		}
	} else if _, err := exec.LookPath("import"); err == nil {
		// ImageMagick import
		cmd = exec.Command("import", "-window", "root", tmpFile)
	} else {
		return ToolResult{Error: "no screenshot tool available on this system"}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("screenshot failed: %s: %s", err, string(output))}
	}

	data, err := exec.Command("cat", tmpFile).Output()
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to read screenshot: %s", err)}
	}
	_ = exec.Command("rm", tmpFile).Run()

	if len(data) == 0 {
		return ToolResult{Error: "screenshot captured but file is empty"}
	}

	base64Data := base64.StdEncoding.EncodeToString(data)

	return ToolResult{
		Output: fmt.Sprintf("Screenshot captured (%s, %d bytes, mode=%s).\nbase64 data follows:\n%s", format, len(data), mode, base64Data),
		Metadata: map[string]any{
			"image_base64": base64Data,
			"image_format": format,
			"image_size":   len(data),
			"mode":         mode,
		},
	}
}

// IsScreenshotResult checks if a tool result contains screenshot metadata
func IsScreenshotResult(result ToolResult) bool {
	_, hasBase64 := result.Metadata["image_base64"]
	return hasBase64
}

// ExtractImageFromResult extracts image data from a tool result
// Returns base64 data and format (png, jpeg, etc.)
func ExtractImageFromResult(result ToolResult) (base64Data, format string, ok bool) {
	if result.Metadata == nil {
		return "", "", false
	}
	base64Data, ok1 := result.Metadata["image_base64"].(string)
	format, ok2 := result.Metadata["image_format"].(string)
	if !ok1 || !ok2 {
		return "", "", false
	}
	return base64Data, format, true
}

// DetectImageFormat detects image format from file extension
func DetectImageFormat(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "jpeg"
	case ".png":
		return "png"
	case ".gif":
		return "gif"
	case ".webp":
		return "webp"
	default:
		return "png"
	}
}