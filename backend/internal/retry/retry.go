package retry

import (
	"context"
	"errors"
	"time"
)

type RetryableError struct {
	Err error
}

func (e RetryableError) Error() string {
	return e.Err.Error()
}

func (e RetryableError) Unwrap() error {
	return e.Err
}

func MarkRetryable(err error) error {
	if err == nil {
		return nil
	}
	return RetryableError{Err: err}
}

func IsRetryable(err error) bool {
	var retryable RetryableError
	return errors.As(err, &retryable)
}

func Do(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if baseDelay <= 0 {
		baseDelay = 200 * time.Millisecond
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if !IsRetryable(err) {
				return err
			}
			if attempt == maxRetries {
				return err
			}
			wait := baseDelay * time.Duration(attempt+1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		return nil
	}
	return lastErr
}
