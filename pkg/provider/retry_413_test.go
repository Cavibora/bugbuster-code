package provider

import (
	"errors"
	"testing"
)

// =============================================================================
// looksLikePayloadTooLarge — detecting an HTTP 413 / "payload_too_large"
// rejection. These are deterministic (the same oversized body is rejected
// again), so the agent must downscale or strip images rather than retry.
// =============================================================================

func TestLooksLikePayloadTooLarge(t *testing.T) {
	real413 := []byte(`{"error":{"message":"request body is too large","type":"invalid_request_error","code":"payload_too_large"}}`)

	tests := []struct {
		name       string
		statusCode int
		body       []byte
		want       bool
	}{
		{"real 413 payload_too_large", 413, real413, true},
		{"413 empty body", 413, []byte(``), true},
		{"413 plain text", 413, []byte(`Content Too Large`), true},
		{"400 wrapped payload_too_large", 400, real413, true},
		{"400 request body is too large", 400, []byte(`{"error":{"message":"request body is too large"}}`), true},
		{"413 image was too large", 413, []byte(`{"error":{"message":"Your image was too large"}}`), true},
		{"size limit exceeded", 400, []byte(`{"error":{"message":"size limit exceeded for request body"}}`), true},
		// Must NOT be misclassified:
		{"genuine invalid_request (no size)", 400, []byte(`{"error":{"type":"invalid_request_error","message":"context_length_exceeded"}}`), false},
		{"content filter", 400, []byte(`{"error":{"message":"content policy violation"}}`), false},
		{"server_error disguised upstream", 400, []byte(`{"error":{"type":"server_error","message":"Upstream request failed"}}`), false},
		{"429 never here", 429, real413, false},
		{"200 never here", 200, real413, false},
		{"503 never here", 503, real413, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikePayloadTooLarge(tt.statusCode, tt.body); got != tt.want {
				t.Errorf("looksLikePayloadTooLarge(%d, %q) = %v, want %v",
					tt.statusCode, tt.body, got, tt.want)
			}
		})
	}
}

// A 413 must surface as a typed ImageTooLargeError, NOT as a generic error or
// a retryable UpstreamError.
func TestFormatHTTPError_PayloadTooLargeBecomesTypedImageError(t *testing.T) {
	body := []byte(`{"error":{"message":"request body is too large","type":"invalid_request_error","code":"payload_too_large"}}`)

	err := FormatHTTPError(413, body)
	if !IsImageTooLargeError(err) {
		t.Fatalf("expected ImageTooLargeError, got %T: %v", err, err)
	}
	if IsUpstreamError(err) {
		t.Error("413 must not be classified as a retryable UpstreamError")
	}
	if IsImageUnsupportedError(err) {
		t.Error("413 must not be classified as ImageUnsupportedError")
	}
	if IsRateLimitError(err) {
		t.Error("413 must not be classified as RateLimitError")
	}

	var itl *ImageTooLargeError
	if !errors.As(err, &itl) {
		t.Fatal("type assertion failed")
	}
	if itl.StatusCode != 413 {
		t.Errorf("expected status 413, got %d", itl.StatusCode)
	}
}

// The text-only "does not support image" 400 must stay ImageUnsupportedError
// and never be mislabeled as a size problem.
func TestFormatHTTPError_ImageUnsupportedNotMistakenForTooLarge(t *testing.T) {
	body := []byte(`{"error":{"message":"model does not support image input","type":"invalid_request_error","code":"invalid_messages"}}`)

	err := FormatHTTPError(400, body)
	if !IsImageUnsupportedError(err) {
		t.Fatalf("expected ImageUnsupportedError, got %T: %v", err, err)
	}
	if IsImageTooLargeError(err) {
		t.Error("image-unsupported must not be classified as ImageTooLargeError")
	}
}
