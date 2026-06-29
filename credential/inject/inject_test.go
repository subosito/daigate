package inject_test

import (
	"net/http"
	"testing"

	"github.com/subosito/daigate/credential/inject"
	"github.com/subosito/daigate/credential/store"
)

func TestStripClientRemovesAuthHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	req.Header.Set("Authorization", "Bearer client")
	req.Header.Set("x-api-key", "client-key")
	inject.StripClient(req)
	if req.Header.Get("Authorization") != "" || req.Header.Get("x-api-key") != "" {
		t.Fatalf("headers not stripped: %+v", req.Header)
	}
}

func TestApplyInjectsBearer(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	req.Header.Set("Authorization", "Bearer client")
	inject.Apply(store.Material{Kind: store.KindAPIKey, APIKey: "sk-up"}, req, "")
	if req.Header.Get("Authorization") != "Bearer sk-up" {
		t.Fatalf("auth=%q", req.Header.Get("Authorization"))
	}
}

func TestApplyInjectsXAPIKey(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	inject.Apply(store.Material{Kind: store.KindAPIKey, APIKey: "cc-key"}, req, "x-api-key")
	if req.Header.Get("x-api-key") != "cc-key" {
		t.Fatalf("x-api-key=%q", req.Header.Get("x-api-key"))
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("authorization should be empty")
	}
}

func TestApplyInjectsCustomHeaderPreset(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	inject.Apply(store.Material{Kind: store.KindAPIKey, APIKey: "el-key"}, req, "xi-api-key")
	if req.Header.Get("xi-api-key") != "el-key" {
		t.Fatalf("xi-api-key=%q", req.Header.Get("xi-api-key"))
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatalf("authorization should be stripped")
	}
}

func TestApplyOAuthPresetRegistered(t *testing.T) {
	inject.RegisterOAuthPreset("anthropic_oauth", func(r *http.Request) {
		r.Header.Set("anthropic-beta", "oauth-2025-04-20")
	})
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	inject.Apply(store.Material{Kind: store.KindOAuth, AccessToken: "oat"}, req, "anthropic_oauth")
	if req.Header.Get("Authorization") != "Bearer oat" {
		t.Fatalf("auth=%q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
		t.Fatalf("beta=%q", req.Header.Get("anthropic-beta"))
	}
}