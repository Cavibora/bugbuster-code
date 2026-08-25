package provider

import (
	"errors"
	"fmt"
	"math"
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
		MaxRetries:      5,
		InitialBackoff:  2 * time.Second,
		MaxBackoff:      60 * time.Second,
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
	if len(bodyStr) > 500 {
		bodyStr = bodyStr[:500] + "..."
	}
	return fmt.Errorf("HTTP %d: %s", statusCode, bodyStr)
}
