package providers

import (
	"net/http"
	"strconv"
	"time"
)

func retryAfterDuration(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if parsed, err := http.ParseTime(value); err == nil {
		wait := time.Until(parsed)
		if wait > 0 {
			return wait
		}
	}
	return 0
}
