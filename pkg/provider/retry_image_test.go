package provider

import (
	"testing"
)

// =============================================================================
// looksLikeImageUnsupported — detecting a text-only endpoint that rejects
// image input (the gufo "model does not support image input" class of errors).
// These are deterministic: the same payload is rejected again, so the agent
// must strip images rather than retry or kill the session.
// =============================================================================

func TestLooksLikeImageUnsupported(t *testing.T) {
	gufoBody := []byte(`{"error":{"message":"model does not support image input","type":"invalid_request_error","code":"invalid_messages"}}`)

	tests := []struct {
		name       string
		statusCode int
		body       []byte
		want       bool
	}{
		{"gufo text-only 400", 400, gufoBody, true},
		{"does not support image", 400, []byte(`{"error":{"message":"this model does not support image input"}}`), true},
		{"not support image plural", 400, []byte(`{"error":{"message":"models do not support image"}}`), true},
		{"image input not supported", 400, []byte(`{"error":{"message":"image input is not supported by this endpoint"}}`), true},
		{"vision not enabled", 400, []byte(`{"error":{"message":"vision is not enabled for this model"}}`), true},
		{"no vision support", 400, []byte(`{"error":{"message":"model has no vision support"}}`), true},
		// Must NOT be misclassified:
		{"content parts format error", 400, []byte(`{"error":{"message":"message content parts must use text or image_url"}}`), false},
		{"genuine invalid_request", 400, []byte(`{"error":{"type":"invalid_request_error","message":"context_length_exceeded"}}`), false},
		{"server_error disguised upstream", 400, []byte(`{"error":{"type":"server_error","message":"Upstream request failed"}}`), false},
		{"plain text 400", 400, []byte(`Bad Request`), false},
		{"429 never here", 429, gufoBody, false},
		{"200 never here", 200, gufoBody, false},
		{"503 never here", 503, gufoBody, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeImageUnsupported(tt.statusCode, tt.body); got != tt.want {
				t.Errorf("looksLikeImageUnsupported(%d, %q) = %v, want %v",
					tt.statusCode, tt.body, got, tt.want)
			}
		})
	}
}

// The gufo error must surface as a typed ImageUnsupportedError, NOT as a
// generic error or an UpstreamError (which would be retried pointlessly).
func TestFormatHTTPError_ImageUnsupportedBecomesTypedImageError(t *testing.T) {
	body := []byte(`{"error":{"message":"model does not support image input","type":"invalid_request_error","code":"invalid_messages"}}`)

	err := FormatHTTPError(400, body)
	if !IsImageUnsupportedError(err) {
		t.Fatalf("expected ImageUnsupportedError, got %T: %v", err, err)
	}
	if IsUpstreamError(err) {
		t.Error("image-unsupported must not be classified as a retryable UpstreamError")
	}
	if IsRateLimitError(err) {
		t.Error("image-unsupported must not be classified as RateLimitError")
	}

	var ie *ImageUnsupportedError
	if !errorsAs(err, &ie) {
		t.Fatal("type assertion failed")
	}
	if ie.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", ie.StatusCode)
	}
}

// errorsAs wraps errors.As for the local type so the test stays self-contained.
func errorsAs(err error, target **ImageUnsupportedError) bool {
	if e, ok := err.(*ImageUnsupportedError); ok {
		*target = e
		return true
	}
	return false
}
