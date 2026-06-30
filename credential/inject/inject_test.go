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

func TestApplyRouteRejectsUnknownInjectPreset(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	mat := store.Material{Kind: store.KindAPIKey, APIKey: "sk-test"}
	err := inject.ApplyRoute(mat, req, inject.Route{Preset: "vendor-custom-preset"}, inject.AdapterDefault{})
	if err == nil {
		t.Fatal("expected error for unknown inject_preset")
	}
}

func TestApplyOAuthPresetRegistered(t *testing.T) {
	inject.RegisterOAuthPreset("vendor_oauth", func(r *http.Request) {
		r.Header.Set("x-vendor-beta", "oauth-2025-04-20")
	})
	req, _ := http.NewRequest(http.MethodPost, "http://example.com", nil)
	inject.Apply(store.Material{Kind: store.KindOAuth, AccessToken: "oat"}, req, "vendor_oauth")
	if req.Header.Get("Authorization") != "Bearer oat" {
		t.Fatalf("auth=%q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("x-vendor-beta") != "oauth-2025-04-20" {
		t.Fatalf("beta=%q", req.Header.Get("x-vendor-beta"))
	}
}