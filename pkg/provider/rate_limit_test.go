package provider

import (
	"testing"
)

func TestRateLimitError_Error(t *testing.T) {
	err := &RateLimitError{
		StatusCode: 429,
		Body:       `{"error":{"message":"rate limited"}}`,
	}
	expected := `HTTP 429: {"error":{"message":"rate limited"}}`
	if err.Error() != expected {
		t.Errorf("RateLimitError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestIsRateLimitError(t *testing.T) {
	rle := &RateLimitError{StatusCode: 429, Body: "rate limited"}
	if !IsRateLimitError(rle) {
		t.Error("IsRateLimitError should return true for *RateLimitError")
	}

	// Regular error should not match
	regularErr := &RetryableError{StatusCode: 500, Body: "server error"}
	if IsRateLimitError(regularErr) {
		t.Error("IsRateLimitError should return false for non-RateLimitError")
	}

	// Nil should not match
	if IsRateLimitError(nil) {
		t.Error("IsRateLimitError should return false for nil")
	}
}

func TestFormatHTTPErrorWithRateLimit_429(t *testing.T) {
	err := FormatHTTPErrorWithRateLimit(429, []byte(`{"error":"rate limited"}`))
	if !IsRateLimitError(err) {
		t.Errorf("FormatHTTPErrorWithRateLimit(429, ...) should return *RateLimitError, got %T", err)
	}
}

func TestFormatHTTPErrorWithRateLimit_500(t *testing.T) {
	err := FormatHTTPErrorWithRateLimit(500, []byte(`{"error":"server error"}`))
	if IsRateLimitError(err) {
		t.Error("FormatHTTPErrorWithRateLimit(500, ...) should NOT return *RateLimitError")
	}
}

func TestFormatHTTPError_429(t *testing.T) {
	err := FormatHTTPError(429, []byte(`{"error":"rate limited"}`))
	if !IsRateLimitError(err) {
		t.Errorf("FormatHTTPError(429, ...) should return *RateLimitError, got %T", err)
	}
}

func TestFormatHTTPError_500(t *testing.T) {
	err := FormatHTTPError(500, []byte(`{"error":"server error"}`))
	if IsRateLimitError(err) {
		t.Error("FormatHTTPError(500, ...) should NOT return *RateLimitError")
	}
}

func TestFormatHTTPErrorWithRateLimit_429_NoTruncation(t *testing.T) {
	longBody := make([]byte, 1000)
	for i := range longBody {
		longBody[i] = 'x'
	}
	err := FormatHTTPErrorWithRateLimit(429, longBody)
	rle, ok := err.(*RateLimitError)
	if !ok {
		t.Fatalf("Expected *RateLimitError, got %T", err)
	}
	// 429 should NOT be truncated — full body preserved
	if len(rle.Body) != 1000 {
		t.Errorf("429 Body should not be truncated, got %d chars, want 1000", len(rle.Body))
	}
}
