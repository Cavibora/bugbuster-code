package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	apiKey := os.Args[1]
	if apiKey == "" {
		fmt.Println("Usage: go run debug_thinking.go <api_key>")
		os.Exit(1)
	}

	baseURL := "https://api.z.ai/api/anthropic"

	// Simple request with thinking
	reqBody := map[string]any{
		"model": "glm-5.1",
		"messages": []map[string]any{
			{"role": "user", "content": "Что такое 2+2? Ответь коротко."},
		},
		"max_tokens": 1024,
		"stream":     true,
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 4096,
		},
	}

	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Headers: %v\n", resp.Header)
	fmt.Println("---RAW SSE DATA---")

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			// Check if there is thinking-related data
			lines := strings.Split(chunk, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "data:") {
					data := strings.TrimPrefix(line, "data: ")
					// Pretty print JSON
					var raw json.RawMessage = json.RawMessage(data)
					if strings.HasPrefix(data, "{") {
						var pretty bytes.Buffer
						if json.Indent(&pretty, raw, "  ", "  ") == nil {
							fmt.Println("  " + pretty.String())
						} else {
							fmt.Println("  " + data)
						}
					} else {
						fmt.Println("  " + data)
					}
				} else if line != "" {
					fmt.Println("  [" + line + "]")
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Read error: %v\n", err)
			break
		}
	}
}
