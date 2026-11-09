package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
)

// RetryPolicy — API retry policy
type RetryPolicy struct {
	MaxRetries      int           `yaml:"max_retries"`      // max retry count (0 = no retries)
	InitialBackoff  time.Duration `yaml:"initial_backoff"`  // initial delay between retries
	MaxBackoff      time.Duration `yaml:"max_backoff"`      // maximum delay between retries
	RetryableErrors []int         `yaml:"retryable_errors"` // HTTP statuses to retry on
}

// DefaultRetryPolicy returns default retry policy.
// Note: 429 is NOT in RetryableErrors — rate limit errors are handled
// by the agent loop (streamRetryRequest) with RateLimitError, which shows
// them to the user and retries with short delays. This prevents double retry.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:      3,
		InitialBackoff:  1 * time.Second,
		MaxBackoff:      30 * time.Second,
		RetryableErrors: []int{500, 502, 503, 504},
	}
}

// NoRetryPolicy returns no-retry policy
func NoRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:      0,
		InitialBackoff:  0,
		MaxBackoff:      0,
		RetryableErrors: nil,
	}
}

// IsRetryable checks, whether to retry request on given error
func (p RetryPolicy) IsRetryable(statusCode int) bool {
	for _, code := range p.RetryableErrors {
		if code == statusCode {
			return true
		}
	}
	return false
}

// BackoffDuration calculates delay for n-th attempt (exponential backoff)
func (p RetryPolicy) BackoffDuration(attempt int) time.Duration {
	if attempt <= 0 {
		return p.InitialBackoff
	}

	// Exponential backoff: initialBackoff * 2^attempt
	duration := p.InitialBackoff * time.Duration(math.Pow(2, float64(attempt)))

	// Limit by maximum delay
	if duration > p.MaxBackoff {
		duration = p.MaxBackoff
	}

	return duration
}

// RetryableError — error that can be retried
type RetryableError struct {
	StatusCode int
	Body       string
	Attempt    int
	MaxRetries int
}

func (e *RetryableError) Error() string {
	return fmt.Sprintf(i18n.T("errors_provider.retry"), e.StatusCode, e.Attempt, e.MaxRetries, e.Body)
}

// RateLimitError — HTTP 429 rate limit error.
// This error is retryable and should NOT be saved to context.
// The agent loop should display it to the user but retry after a delay.
type RateLimitError struct {
	StatusCode int
	Body       string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// IsRateLimitError checks if an error is a RateLimitError.
func IsRateLimitError(err error) bool {
	var rle *RateLimitError
	return errors.As(err, &rle)
}

// UpstreamError — a transient upstream/model failure that some gateways
// (e.g. OpenCode Zen) wrap in a client-looking HTTP 4xx status. The body
// carries type/code "server_error" and a message like "Upstream request
// failed: ... Upstream response was not valid JSON". This is NOT a client
// error: the same request usually succeeds on retry, so it must be retried
// like a 5xx.
type UpstreamError struct {
	StatusCode int
	Body       string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// IsUpstreamError checks if an error is a disguised upstream failure.
func IsUpstreamError(err error) bool {
	var ue *UpstreamError
	return errors.As(err, &ue)
}

// ImageUnsupportedError — a deterministic client error returned when a
// request carries image content but the endpoint/model cannot accept image
// input (e.g. a text-only OpenAI-compatible server rejecting image_url parts
// with "model does not support image input"). Unlike UpstreamError this will
// NEVER succeed on retry with the same payload: the offending images must be
// removed from the conversation first. Surfacing it as a distinct type lets
// the agent self-heal (strip images and resend) instead of killing the session
// — otherwise the stale image blocks stay in history and poison every
// subsequent message forever.
type ImageUnsupportedError struct {
	StatusCode int
	Body       string
}

func (e *ImageUnsupportedError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// IsImageUnsupportedError checks if an error means the endpoint rejected
// image input.
func IsImageUnsupportedError(err error) bool {
	var ie *ImageUnsupportedError
	return errors.As(err, &ie)
}

// ImageTooLargeError — a deterministic client error returned when the request
// body exceeds the provider/gateway size limit (HTTP 413 "payload_too_large",
// or a 4xx whose message complains about request/body/image size). Unlike a
// transient UpstreamError this never succeeds on retry with the same payload:
// the offending images must be downscaled or removed from the conversation
// first. Surfacing it as a distinct type lets the agent self-heal (drop or
// shrink the oversized images and resend) instead of killing the session —
// otherwise a large screenshot stays in history and poisons every later
// message.
type ImageTooLargeError struct {
	StatusCode int
	Body       string
}

func (e *ImageTooLargeError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// IsImageTooLargeError checks if an error means the request was rejected for
// being too large (typically because of image payloads).
func IsImageTooLargeError(err error) bool {
	var ie *ImageTooLargeError
	return errors.As(err, &ie)
}

// looksLikePayloadTooLarge inspects a 4xx to detect a request rejected because
// its body is too large. The canonical signal is HTTP 413, but some gateways
// wrap the same condition in a 400 with an explicit "payload_too_large" code
// or a "request body is too large" message. We require the wording to be about
// the *size* of the request/body/payload (not, say, a context-length overflow,
// which is handled elsewhere), so a genuine unrelated 400 is never
// misclassified.
func looksLikePayloadTooLarge(statusCode int, body []byte) bool {
	// Only 4xx client errors are relevant. 5xx are retryable server errors,
	// 429 is a rate limit, and 2xx succeeded — none of them is a size
	// rejection, so a body that happens to mention "payload_too_large" under
	// those statuses must not be misclassified.
	if statusCode < 400 || statusCode >= 500 || statusCode == 429 {
		return false
	}
	lower := strings.ToLower(string(body))
	// Explicit size codes/messages that any 4xx may carry.
	if strings.Contains(lower, "payload_too_large") ||
		strings.Contains(lower, "request body is too large") ||
		strings.Contains(lower, "request too large") ||
		strings.Contains(lower, "body too large") ||
		strings.Contains(lower, "image was too large") ||
		strings.Contains(lower, "image too large") ||
		strings.Contains(lower, "size limit exceeded") {
		return true
	}
	// HTTP 413 (Content Too Large) is unambiguous even with a terse body.
	if statusCode == 413 {
		return true
	}
	return false
}

// looksLikeImageUnsupported inspects a 4xx body to detect a server that
// rejects image input. It matches the wording used by text-only
// OpenAI-compatible gateways ("model does not support image input",
// "does not accept image", "image input is not supported", ...) and the
// Anthropic-style "unsupported ... image". We only treat it as image-related
// when the message explicitly couples "image" with "not support/accept" or
// "invalid ... image", so a genuine invalid_request unrelated to images is
// never misclassified.
func looksLikeImageUnsupported(statusCode int, body []byte) bool {
	if statusCode < 400 || statusCode >= 500 || statusCode == 429 {
		return false
	}
	lower := strings.ToLower(string(body))
	// Only image/vision capability rejections are relevant here.
	if !strings.Contains(lower, "image") && !strings.Contains(lower, "vision") {
		return false
	}
	// Coupling of "image" with a rejection verb/phrase.
	if strings.Contains(lower, "does not support image") ||
		strings.Contains(lower, "do not support image") ||
		strings.Contains(lower, "not support image") ||
		strings.Contains(lower, "unsupported image") ||
		strings.Contains(lower, "does not accept image") ||
		strings.Contains(lower, "cannot accept image") ||
		strings.Contains(lower, "invalid image") {
		return true
	}
	// "image input ..." + a rejection word (e.g. "image input is not supported").
	if strings.Contains(lower, "image input") &&
		(strings.Contains(lower, "not support") ||
			strings.Contains(lower, "unsupported") ||
			strings.Contains(lower, "not accept") ||
			strings.Contains(lower, "invalid")) {
		return true
	}
	// Vision-capability wording: "model has no vision support", "vision not enabled".
	if strings.Contains(lower, "vision") &&
		(strings.Contains(lower, "not") || strings.Contains(lower, "no ")) {
		return true
	}
	return false
}

// looksLikeUpstreamError inspects an HTTP error body to detect a transient
// upstream failure that was mislabeled with a 4xx status. Gateways return
// error.type/error.code == "server_error" (or an "upstream" message) when the
// model behind them hiccuped — retrying is correct, surfacing it is not.
//
// Returns false for genuine client errors (invalid_request, context_length,
// content_filter, etc.) so we never retry a request the server will reject
// again.
func looksLikeUpstreamError(statusCode int, body []byte) bool {
	// 5xx are already retryable via RetryableErrors; 429 is RateLimitError.
	// This detector is only for 4xx that disguise a server-side failure.
	if statusCode < 400 || statusCode >= 500 || statusCode == 429 {
		return false
	}

	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		// Not JSON — fall back to a substring scan.
		lower := strings.ToLower(string(body))
		return strings.Contains(lower, "upstream") &&
			strings.Contains(lower, "server_error")
	}

	et := strings.ToLower(payload.Error.Type)
	ec := strings.ToLower(payload.Error.Code)
	em := strings.ToLower(payload.Error.Message)

	// Explicit server_error type/code on a 4xx = disguised upstream failure.
	if et == "server_error" || ec == "server_error" {
		return true
	}
	// "Upstream ... failed" wording without a server_error tag.
	if strings.Contains(em, "upstream") &&
		(strings.Contains(em, "not valid json") ||
			strings.Contains(em, "request failed") ||
			strings.Contains(em, "bad gateway") ||
			strings.Contains(em, "timeout") ||
			strings.Contains(em, "overloaded")) {
		return true
	}
	return false
}

// IsRetryableError checks if an error is a RetryableError.
func IsRetryableError(err error) bool {
	var re *RetryableError
	return errors.As(err, &re)
}

// FormatHTTPErrorWithRateLimit formats HTTP error and returns RateLimitError for 429.
// For other status codes returns a regular error.
// For 429, the full error body is preserved (not truncated) so the user
// can see the complete error message from the provider.
func FormatHTTPErrorWithRateLimit(statusCode int, body []byte) error {
	bodyStr := string(body)
	if statusCode == 429 {
		return &RateLimitError{
			StatusCode: statusCode,
			Body:       bodyStr,
		}
	}
	// Some gateways (OpenCode Zen) disguise a transient upstream failure as
	// a 4xx. Surface it as UpstreamError so callers retry instead of failing.
	if looksLikeUpstreamError(statusCode, body) {
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "..."
		}
		return &UpstreamError{
			StatusCode: statusCode,
			Body:       bodyStr,
		}
	}
	// A text-only endpoint rejecting image input is deterministic — surface it
	// as ImageUnsupportedError so the agent strips images and resends instead
	// of retrying a payload the server will always reject (and instead of
	// killing the session, which would leave stale images poisoning history).
	if looksLikeImageUnsupported(statusCode, body) {
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "..."
		}
		return &ImageUnsupportedError{
			StatusCode: statusCode,
			Body:       bodyStr,
		}
	}
	// The request was rejected for being too large — almost always because of
	// image payloads (a Retina screenshot alone can exceed a gateway's body
	// cap). Surface it as ImageTooLargeError so the agent downscales/strips
	// images and resends instead of retrying a payload that will always be
	// rejected (and instead of killing the session, which would leave the
	// oversized image poisoning history). Checked after looksLikeImageUnsupported
	// so a text-only "does not support image" is never mislabeled as a size
	// problem.
	if looksLikePayloadTooLarge(statusCode, body) {
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "..."
		}
		return &ImageTooLargeError{
			StatusCode: statusCode,
			Body:       bodyStr,
		}
	}
	if len(bodyStr) > 500 {
		bodyStr = bodyStr[:500] + "..."
	}
	return fmt.Errorf("HTTP %d: %s", statusCode, bodyStr)
}
