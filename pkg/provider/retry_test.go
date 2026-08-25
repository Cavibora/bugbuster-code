package provider

import (
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", policy.MaxRetries)
	}
	if policy.InitialBackoff != 2*time.Second {
		t.Errorf("Expected InitialBackoff=2s, got %v", policy.InitialBackoff)
	}
	if policy.MaxBackoff != 60*time.Second {
		t.Errorf("Expected MaxBackoff=60s, got %v", policy.MaxBackoff)
	}
	// 429 is NOT in RetryableErrors — it's handled by streamRetryRequest with RateLimitError
	if len(policy.RetryableErrors) != 4 {
		t.Errorf("Expected 4 retryable errors (no 429), got %d", len(policy.RetryableErrors))
	}
	for _, code := range policy.RetryableErrors {
		if code == 429 {
			t.Error("429 should NOT be in RetryableErrors — handled by streamRetryRequest")
		}
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
		{429, false}, // NOT retryable — handled by streamRetryRequest with RateLimitError
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
		{0, 2 * time.Second, 3 * time.Second},    // 2s
		{1, 4 * time.Second, 5 * time.Second},    // 4s
		{2, 8 * time.Second, 9 * time.Second},    // 8s
		{3, 16 * time.Second, 17 * time.Second},  // 16s
		{4, 32 * time.Second, 33 * time.Second}, // 32s
		{10, 60 * time.Second, 60 * time.Second}, // capped at 60s
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
	if provider.retryPolicy.MaxRetries != 5 {
		t.Errorf("Expected default MaxRetries=5, got %d", provider.retryPolicy.MaxRetries)
	}

	// Custom retry policy
	customPolicy := NoRetryPolicy()
	provider.SetRetryPolicy(customPolicy)

	if provider.retryPolicy.MaxRetries != 0 {
		t.Errorf("Expected custom MaxRetries=0, got %d", provider.retryPolicy.MaxRetries)
	}
}
