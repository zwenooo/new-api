package middleware

import (
	"net/http"
	"testing"
)

func TestOpenAIErrorTypeForStatus(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		want       string
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, want: "invalid_request_error"},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, want: "authentication_error"},
		{name: "forbidden", statusCode: http.StatusForbidden, want: "permission_error"},
		{name: "rate limit", statusCode: http.StatusTooManyRequests, want: "rate_limit_error"},
		{name: "server error", statusCode: http.StatusServiceUnavailable, want: "transfer_api_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := openAIErrorTypeForStatus(tc.statusCode); got != tc.want {
				t.Fatalf("openAIErrorTypeForStatus(%d) = %q, want %q", tc.statusCode, got, tc.want)
			}
		})
	}
}
