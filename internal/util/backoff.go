package util

import "time"

// Backoff returns exponential backoff delay for attempt number (0-based).
func Backoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := 200 * time.Millisecond
	for i := 0; i < attempt; i++ {
		d *= 2
		if d > 5*time.Second {
			return 5 * time.Second
		}
	}
	return d
}
