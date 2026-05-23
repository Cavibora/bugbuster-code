package provider

import (
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

// DefaultRetryPolicy returns default retry policy
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:      3,
		InitialBackoff:  1 * time.Second,
		MaxBackoff:      30 * time.Second,
		RetryableErrors: []int{429, 500, 502, 503, 504},
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
