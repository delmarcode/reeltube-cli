package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	maxRetries     = 3
	initialBackoff = 100 * time.Millisecond
	maxBackoff     = 5 * time.Second
)

// IsRetryableError returns true if the error or status code should trigger a retry
func IsRetryableError(err error, statusCode int) bool {
	// Network errors are retryable
	if err != nil {
		// Check for context cancellation - not retryable
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		// Other network errors are retryable
		return true
	}
	// 5xx server errors are retryable
	return statusCode >= 500 && statusCode < 600
}

// UploadPartWithRetry uploads a single part to S3 with exponential backoff retry
func UploadPartWithRetry(ctx context.Context, url string, data []byte) (*http.Response, error) {
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Wait with exponential backoff, respecting context cancellation
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}

		req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(data))
		if err != nil {
			return nil, err // Request creation errors are not retryable
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if IsRetryableError(err, 0) && attempt < maxRetries {
				lastErr = err
				continue
			}
			return nil, err
		}

		// Check for retryable status codes (5xx server errors)
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			resp.Body.Close()
			if attempt < maxRetries {
				lastErr = fmt.Errorf("server error: status %d", resp.StatusCode)
				continue
			}
			// Max retries exceeded with server error
			return nil, fmt.Errorf("max retries exceeded: server error status %d", resp.StatusCode)
		}

		// 4xx client errors are not retryable - fail immediately
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			resp.Body.Close()
			return nil, fmt.Errorf("client error: status %d (not retryable)", resp.StatusCode)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}
