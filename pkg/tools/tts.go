package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TTSTool converts text to speech using OpenAI TTS API or system TTS
type TTSTool struct {
	APIKey     string
	BaseURL    string
	Model      string // TTS model: "tts-1" or "tts-1-hd"
	Voice      string // Voice: alloy, echo, fable, onyx, nova, shimmer
	httpClient *http.Client
}

// NewTTSTool creates a new TTS tool
func NewTTSTool(apiKey, baseURL string) *TTSTool {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &TTSTool{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   "tts-1",
		Voice:   "alloy",
		httpClient: &http.Client{},
	}
}

func (t *TTSTool) Name() string {
	return "tts"
}

func (t *TTSTool) Description() string {
	return "Convert text to speech audio. Uses OpenAI TTS API if available, otherwise falls back to system TTS (say on macOS). Returns path to audio file."
}

func (t *TTSTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to convert to speech",
			},
			"voice": map[string]any{
				"type":        "string",
				"description": "Voice to use: 'alloy', 'echo', 'fable', 'onyx', 'nova', 'shimmer' (OpenAI) or system voice name",
				"enum":        []string{"alloy", "echo", "fable", "onyx", "nova", "shimmer"},
				"default":     "alloy",
			},
			"output": map[string]any{
				"type":        "string",
				"description": "Output file path (default: /tmp/tts_output.mp3)",
			},
		},
		"required": []string{"text"},
	}
}

func (t *TTSTool) Execute(params map[string]string) ToolResult {
	text := params["text"]
	if text == "" {
		return ToolResult{Error: "text parameter is required"}
	}

	voice := params["voice"]
	if voice == "" {
		voice = t.Voice
	}

	outputPath := params["output"]
	if outputPath == "" {
		outputPath = filepath.Join(os.TempDir(), "tts_output.mp3")
	}

	// Try OpenAI TTS API first
	if t.APIKey != "" {
		result, err := t.openaiTTS(text, voice, outputPath)
		if err == nil {
			return result
		}
		// Fall through to system TTS
	}

	// Fall back to system TTS (macOS 'say' command)
	return t.systemTTS(text, voice, outputPath)
}

func (t *TTSTool) openaiTTS(text, voice, outputPath string) (ToolResult, error) {
	reqBody := map[string]any{
		"model":          t.Model,
		"input":          text,
		"voice":          voice,
		"response_format": "mp3",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", t.BaseURL+"/audio/speech", strings.NewReader(string(body)))
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return ToolResult{}, fmt.Errorf("TTS API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return ToolResult{}, fmt.Errorf("TTS API error %d: %s", resp.StatusCode, string(respBody))
	}

	// Save audio to file
	f, err := os.Create(outputPath)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to write audio: %w", err)
	}

	// Auto-play on macOS
	go t.playAudio(outputPath)

	return ToolResult{
		Output: fmt.Sprintf("Speech generated: %s (%d bytes, voice=%s)\nText: %s", outputPath, written, voice, truncate(text, 200)),
	}, nil
}

func (t *TTSTool) systemTTS(text, voice, outputPath string) ToolResult {
	// macOS 'say' command
	if _, err := exec.LookPath("say"); err == nil {
		// Generate AIFF first, then convert to MP3 if ffmpeg available
		aiffPath := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".aiff"

		args := []string{"-o", aiffPath}
		// macOS voices: Alex, Fred, Victoria, etc.
		if voice != "" && voice != "alloy" {
			// Map OpenAI voice names to macOS voices
			macVoice := mapOpenAIVoiceToMac(voice)
			args = append(args, "-v", macVoice)
		}
		args = append(args, text)

		cmd := exec.Command("say", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return ToolResult{Error: fmt.Sprintf("system TTS failed: %s: %s", err, string(output))}
		}

		// Try to convert to MP3 with ffmpeg
		if _, err := exec.LookPath("ffmpeg"); err == nil {
			cmd := exec.Command("ffmpeg", "-y", "-i", aiffPath, "-codec:a", "libmp3lame", "-qscale:a", "2", outputPath)
			if output, err := cmd.CombinedOutput(); err != nil {
				// ffmpeg failed, use AIFF
				outputPath = aiffPath
				_ = fmt.Sprintf("ffmpeg conversion failed: %s: %s", err, string(output))
			} else {
				_ = os.Remove(aiffPath)
			}
		} else {
			// No ffmpeg, use AIFF
			outputPath = aiffPath
		}

		// Auto-play
		go t.playAudio(outputPath)

		return ToolResult{
			Output: fmt.Sprintf("Speech generated (system TTS): %s\nText: %s", outputPath, truncate(text, 200)),
		}
	}

	// Try espeak on Linux
	if _, err := exec.LookPath("espeak"); err == nil {
		cmd := exec.Command("espeak", "-w", outputPath, text)
		if output, err := cmd.CombinedOutput(); err != nil {
			return ToolResult{Error: fmt.Sprintf("espeak failed: %s: %s", err, string(output))}
		}
		go t.playAudio(outputPath)
		return ToolResult{
			Output: fmt.Sprintf("Speech generated (espeak): %s\nText: %s", outputPath, truncate(text, 200)),
		}
	}

	return ToolResult{Error: "no TTS available: install OpenAI API key, or 'say' (macOS) / 'espeak' (Linux)"}
}

func (t *TTSTool) playAudio(path string) {
	// macOS
	if _, err := exec.LookPath("afplay"); err == nil {
		_ = exec.Command("afplay", path).Run()
		return
	}
	// Linux
	if _, err := exec.LookPath("aplay"); err == nil {
		_ = exec.Command("aplay", path).Run()
		return
	}
	if _, err := exec.LookPath("mpv"); err == nil {
		_ = exec.Command("mpv", "--no-video", path).Run()
		return
	}
}

func mapOpenAIVoiceToMac(voice string) string {
	mapping := map[string]string{
		"alloy":   "Alex",
		"echo":    "Fred",
		"fable":   "Victoria",
		"onyx":    "Daniel",
		"nova":    "Samantha",
		"shimmer": "Karen",
	}
	if macVoice, ok := mapping[voice]; ok {
		return macVoice
	}
	return voice
}