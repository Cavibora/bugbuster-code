package main

import (
	"errors"
	"testing"
)

func TestIsTransientHTTPError_DisguisedUpstream(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "opencode disguised 400 server_error",
			err:  errors.New(`HTTP 400: {"error":{"type":"server_error","code":"server_error","message":"Upstream request failed: [server_error] Upstream response was not valid JSON"}}`),
			want: true,
		},
		{
			name: "upstream not valid json",
			err:  errors.New("HTTP 400: Upstream response was not valid JSON"),
			want: true,
		},
		{
			name: "rate limited 429",
			err:  errors.New("HTTP 429: rate limited"),
			want: true,
		},
		{
			name: "genuine invalid_request 400",
			err:  errors.New(`HTTP 400: {"error":{"type":"invalid_request_error","message":"context length exceeded"}}`),
			want: false,
		},
		{
			name: "auth error 401",
			err:  errors.New("HTTP 401: unauthorized"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientHTTPError(tc.err); got != tc.want {
				t.Errorf("isTransientHTTPError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
