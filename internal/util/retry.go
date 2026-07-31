package util

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries  int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	RetryOn     func(error) bool // Optional: custom retry predicate
}

// DefaultRetryConfig returns sensible defaults for API calls.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
	}
}

// WithRetry executes fn with exponential backoff and jitter.
func WithRetry(ctx context.Context, cfg RetryConfig, operation string, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(float64(cfg.BaseDelay) * math.Pow(2, float64(attempt-1)))
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
			// Add jitter: 50-100% of delay
			jitter := time.Duration(rand.Int63n(int64(delay / 2)))
			delay = delay/2 + jitter

			slog.Warn("retrying operation", "operation", operation, "attempt", attempt, "delay", delay)

			select {
			case <-ctx.Done():
				return fmt.Errorf("%s: context cancelled during retry: %w", operation, ctx.Err())
			case <-time.After(delay):
			}
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if cfg.RetryOn != nil && !cfg.RetryOn(lastErr) {
			return lastErr
		}

		slog.Warn("operation failed", "operation", operation, "attempt", attempt, "error", lastErr)
	}
	return fmt.Errorf("%s: all %d retries exhausted: %w", operation, cfg.MaxRetries, lastErr)
}

// IsRetryableHTTPStatus checks if an HTTP status code is retryable.
func IsRetryableHTTPStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout ||
		statusCode == http.StatusBadGateway ||
		statusCode >= 500
}
