package upstream_test

import (
	"testing"

	"github.com/subosito/daigate/upstream"
)

func TestJoinURL(t *testing.T) {
	tests := []struct {
		base, path, want string
	}{
		{
			base: "https://api.example.com",
			path: "/v1/chat/completions",
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			base: "https://host.example/v1",
			path: "/v1/chat/completions",
			want: "https://host.example/v1/chat/completions",
		},
		{
			base: "https://compat.example/v1",
			path: "/v1/chat/completions",
			want: "https://compat.example/v1/chat/completions",
		},
		{
			base: "https://api.anthropic.com",
			path: "/v1/messages",
			want: "https://api.anthropic.com/v1/messages",
		},
		{
			base: "https://api.example/v1/",
			path: "/v1/responses",
			want: "https://api.example/v1/responses",
		},
		{
			base: "https://embed.example/compatible-mode/v1",
			path: "/v1/embeddings",
			want: "https://embed.example/compatible-mode/v1/embeddings",
		},
	}
	for _, tc := range tests {
		if got := upstream.JoinURL(tc.base, tc.path); got != tc.want {
			t.Fatalf("JoinURL(%q, %q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}