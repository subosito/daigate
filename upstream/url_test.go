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

func TestImageUpstreamPath(t *testing.T) {
	tests := []struct {
		base, ingress, want string
	}{
		{
			base:    "https://api.x.ai/v1/images",
			ingress: "/v1/images/generations",
			want:    "https://api.x.ai/v1/images/generations",
		},
		{
			base:    "https://api.openai.com/v1/images",
			ingress: "/v1/images/edits",
			want:    "https://api.openai.com/v1/images/edits",
		},
		{
			base:    "https://api.tokenrouter.com",
			ingress: "/v1/images/generations",
			want:    "https://api.tokenrouter.com/v1/images/generations",
		},
	}
	for _, tc := range tests {
		if got := upstream.ImageUpstreamPath(tc.base, tc.ingress); got != tc.want {
			t.Fatalf("ImageUpstreamPath(%q, %q) = %q, want %q", tc.base, tc.ingress, got, tc.want)
		}
	}
}

func TestVideoUpstreamPath(t *testing.T) {
	tests := []struct {
		base, ingress, want string
	}{
		{
			base:    "https://api.x.ai/v1/videos",
			ingress: "/v1/videos/generations",
			want:    "https://api.x.ai/v1/videos/generations",
		},
		{
			base:    "https://api.x.ai/v1/videos",
			ingress: "/v1/videos/vid_abc123",
			want:    "https://api.x.ai/v1/videos/vid_abc123",
		},
		{
			base:    "https://api.example.com",
			ingress: "/v1/videos/generations",
			want:    "https://api.example.com/v1/videos/generations",
		},
	}
	for _, tc := range tests {
		if got := upstream.VideoUpstreamPath(tc.base, tc.ingress); got != tc.want {
			t.Fatalf("VideoUpstreamPath(%q, %q) = %q, want %q", tc.base, tc.ingress, got, tc.want)
		}
	}
}