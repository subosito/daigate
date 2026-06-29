package upstream_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/subosito/daigate/upstream"
)

func TestRetryable(t *testing.T) {
	cases := []struct {
		status int
		err    error
		want   bool
	}{
		{0, errors.New("dial"), true},
		{http.StatusOK, nil, false},
		{http.StatusBadRequest, nil, false},
		{http.StatusTooManyRequests, nil, true},
		{http.StatusBadGateway, nil, true},
		{http.StatusServiceUnavailable, nil, true},
		{http.StatusGatewayTimeout, nil, true},
	}
	for _, tc := range cases {
		if got := upstream.Retryable(tc.status, tc.err); got != tc.want {
			t.Fatalf("status=%d err=%v got=%v want=%v", tc.status, tc.err, got, tc.want)
		}
	}
}