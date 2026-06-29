package upstream

import (
	"net/http"
)

// Retryable reports whether another pool member may be tried.
func Retryable(status int, err error) bool {
	if err != nil {
		return true
	}
	switch status {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}