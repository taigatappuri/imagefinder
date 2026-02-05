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

type RetryAfter interface {
	RetryAfter() time.Duration
}

type RetryAfterError struct {
	Err   error
	Delay time.Duration
}

func (e RetryAfterError) Error() string {
	return e.Err.Error()
}

func (e RetryAfterError) Unwrap() error {
	return e.Err
}

func (e RetryAfterError) RetryAfter() time.Duration {
	return e.Delay
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
			if retryAfter, ok := retryAfterDuration(err); ok && retryAfter > wait {
				wait = retryAfter
			}
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

func retryAfterDuration(err error) (time.Duration, bool) {
	var retryAfter RetryAfter
	if errors.As(err, &retryAfter) {
		return retryAfter.RetryAfter(), true
	}
	return 0, false
}
