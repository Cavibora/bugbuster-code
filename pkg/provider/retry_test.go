package provider

import (
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", policy.MaxRetries)
	}
	if policy.InitialBackoff != 1*time.Second {
		t.Errorf("Expected InitialBackoff=1s, got %v", policy.InitialBackoff)
	}
	if policy.MaxBackoff != 30*time.Second {
		t.Errorf("Expected MaxBackoff=30s, got %v", policy.MaxBackoff)
	}
	if len(policy.RetryableErrors) != 5 {
		t.Errorf("Expected 5 retryable errors, got %d", len(policy.RetryableErrors))
	}
}

func TestNoRetryPolicy(t *testing.T) {
	policy := NoRetryPolicy()

	if policy.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries=0, got %d", policy.MaxRetries)
	}
	if policy.InitialBackoff != 0 {
		t.Errorf("Expected InitialBackoff=0, got %v", policy.InitialBackoff)
	}
}

func TestIsRetryable(t *testing.T) {
	policy := DefaultRetryPolicy()

	tests := []struct {
		code     int
		expected bool
	}{
		{429, true},  // Too Many Requests
		{500, true},  // Internal Server Error
		{502, true},  // Bad Gateway
		{503, true},  // Service Unavailable
		{504, true},  // Gateway Timeout
		{400, false}, // Bad Request
		{401, false}, // Unauthorized
		{403, false}, // Forbidden
		{404, false}, // Not Found
		{200, false}, // OK
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.code)), func(t *testing.T) {
			result := policy.IsRetryable(tt.code)
			if result != tt.expected {
				t.Errorf("IsRetryable(%d) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestBackoffDuration(t *testing.T) {
	policy := DefaultRetryPolicy()

	tests := []struct {
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{0, 1 * time.Second, 2 * time.Second},    // 1s
		{1, 2 * time.Second, 3 * time.Second},    // 2s
		{2, 4 * time.Second, 5 * time.Second},    // 4s
		{3, 8 * time.Second, 9 * time.Second},    // 8s
		{4, 16 * time.Second, 17 * time.Second},  // 16s
		{10, 30 * time.Second, 30 * time.Second}, // capped at 30s
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.attempt+'0')), func(t *testing.T) {
			duration := policy.BackoffDuration(tt.attempt)
			if duration < tt.minDelay || duration > tt.maxDelay {
				t.Errorf("BackoffDuration(%d) = %v, want between %v and %v",
					tt.attempt, duration, tt.minDelay, tt.maxDelay)
			}
		})
	}
}

func TestRetryableError(t *testing.T) {
	err := &RetryableError{
		StatusCode: 429,
		Body:       "rate limit exceeded",
		Attempt:    1,
		MaxRetries: 3,
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("RetryableError.Error() should return non-empty string")
	}
}

func TestOpenAIProvider_SetRetryPolicy(t *testing.T) {
	provider, err := NewOpenAIProvider("test", ProviderConfig{
		Type:  "openai",
		Model: "gpt-4o",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Default retry policy
	if provider.retryPolicy.MaxRetries != 3 {
		t.Errorf("Expected default MaxRetries=3, got %d", provider.retryPolicy.MaxRetries)
	}

	// Custom retry policy
	customPolicy := NoRetryPolicy()
	provider.SetRetryPolicy(customPolicy)

	if provider.retryPolicy.MaxRetries != 0 {
		t.Errorf("Expected custom MaxRetries=0, got %d", provider.retryPolicy.MaxRetries)
	}
}
