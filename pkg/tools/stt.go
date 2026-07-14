package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// STTTool converts speech to text using OpenAI Whisper API or system tools
type STTTool struct {
	APIKey     string
	BaseURL    string
	Model      string // STT model: "whisper-1"
	Language    string // Language hint: "en", "ru", etc.
	httpClient *http.Client
}

// NewSTTTool creates a new STT tool
func NewSTTTool(apiKey, baseURL string) *STTTool {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &STTTool{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		Model:      "whisper-1",
		Language:   "",
		httpClient: &http.Client{},
	}
}

func (t *STTTool) Name() string {
	return "stt"
}

func (t *STTTool) Description() string {
	return "Convert speech to text (transcription). Records audio from microphone or transcribes an audio file. Uses OpenAI Whisper API if available, otherwise falls back to system tools."
}

func (t *STTTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{
				"type":        "string",
				"description": "Path to audio file to transcribe. If empty, records from microphone (3 seconds).",
			},
			"duration": map[string]any{
				"type":        "string",
				"description": "Recording duration when recording from microphone (e.g., '3s', '10s', '30s'). Default: '5s'",
				"default":     "5s",
			},
			"language": map[string]any{
				"type":        "string",
				"description": "Language hint for transcription (e.g., 'en', 'ru', 'de'). Empty = auto-detect.",
			},
		},
		"required": []string{},
	}
}

func (t *STTTool) Execute(params map[string]string) ToolResult {
	filePath := params["file"]
	language := params["language"]
	if language == "" {
		language = t.Language
	}
	duration := params["duration"]
	if duration == "" {
		duration = "5s"
	}

	// If no file provided, record from microphone
	if filePath == "" {
		var err error
		filePath, err = t.recordFromMicrophone(duration)
		if err != nil {
			return ToolResult{Error: fmt.Sprintf("failed to record audio: %s", err)}
		}
		defer os.Remove(filePath) // Clean up temp file
	}

	// Check file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return ToolResult{Error: fmt.Sprintf("file not found: %s", filePath)}
	}

	// Try OpenAI Whisper API first
	if t.APIKey != "" {
		result, err := t.whisperTranscribe(filePath, language)
		if err == nil {
			return result
		}
		// Fall through to system tools
	}

	// Try local whisper CLI
	if _, err := exec.LookPath("whisper"); err == nil {
		return t.localWhisper(filePath, language)
	}

	return ToolResult{Error: "no STT available: install OpenAI API key or 'whisper' CLI"}
}

func (t *STTTool) recordFromMicrophone(duration string) (string, error) {
	// Parse duration
	seconds := "5"
	if strings.HasSuffix(duration, "s") {
		seconds = strings.TrimSuffix(duration, "s")
	}

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("stt_recording_%d.wav", os.Getpid()))

	// macOS: use 'rec' from SoX or 'afrecord'
	if _, err := exec.LookPath("rec"); err == nil {
		// SoX rec command
		cmd := exec.Command("rec", "-r", "16000", "-c", "1", tmpFile, "trim", "0", seconds)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("recording failed: %s: %s", err, string(output))
		}
		return tmpFile, nil
	}

	// macOS: use afrecord
	if _, err := exec.LookPath("afrecord"); err == nil {
		cmd := exec.Command("afrecord", "-f", "WAVE", "-r", "16000", "-c", "1", "-t", seconds, tmpFile)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("recording failed: %s: %s", err, string(output))
		}
		return tmpFile, nil
	}

	// Try ffmpeg with avfoundation (macOS)
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		cmd := exec.Command("ffmpeg", "-y", "-f", "avfoundation", "-i", ":0",
			"-t", seconds, "-ar", "16000", "-ac", "1", tmpFile)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("recording failed: %s: %s", err, string(output))
		}
		return tmpFile, nil
	}

	// Try arecord on Linux
	if _, err := exec.LookPath("arecord"); err == nil {
		cmd := exec.Command("arecord", "-f", "S16_LE", "-r", "16000", "-c", "1",
			"-d", seconds, tmpFile)
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("recording failed: %s: %s", err, string(output))
		}
		return tmpFile, nil
	}

	return "", fmt.Errorf("no audio recording tool found (install SoX, ffmpeg, or arecord)")
}

func (t *STTTool) whisperTranscribe(filePath, language string) (ToolResult, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to read audio file: %w", err)
	}

	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file
	ext := filepath.Ext(filePath)
	part, err := writer.CreateFormFile("file", "audio"+ext)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return ToolResult{}, fmt.Errorf("failed to write file data: %w", err)
	}

	// Add model
	_ = writer.WriteField("model", t.Model)

	// Add language if specified
	if language != "" {
		_ = writer.WriteField("language", language)
	}

	writer.Close()

	// Send request
	req, err := http.NewRequest("POST", t.BaseURL+"/audio/transcriptions", &buf)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.APIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return ToolResult{}, fmt.Errorf("Whisper API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return ToolResult{}, fmt.Errorf("Whisper API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return ToolResult{}, fmt.Errorf("failed to parse response: %w", err)
	}

	return ToolResult{
		Output: fmt.Sprintf("Transcription:\n%s", result.Text),
	}, nil
}

func (t *STTTool) localWhisper(filePath, language string) ToolResult {
	args := []string{filePath, "--model", "base"}
	if language != "" {
		args = append(args, "--language", language)
	}

	cmd := exec.Command("whisper", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("local whisper failed: %s: %s", err, string(output))}
	}

	return ToolResult{
		Output: fmt.Sprintf("Transcription (local whisper):\n%s", string(output)),
	}
}