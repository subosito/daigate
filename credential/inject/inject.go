package inject

import (
	"net/http"
	"strings"
	"sync"

	"github.com/subosito/daigate/credential/store"
)

// OAuthPresetFunc applies extra OAuth headers after Bearer is set.
type OAuthPresetFunc func(r *http.Request)

var (
	oauthPresetMu sync.RWMutex
	oauthPresets  = map[string]OAuthPresetFunc{}
)

// RegisterOAuthPreset registers extension OAuth header shaping (e.g. anthropic_oauth).
func RegisterOAuthPreset(name string, fn OAuthPresetFunc) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" || fn == nil {
		return
	}
	oauthPresetMu.Lock()
	defer oauthPresetMu.Unlock()
	oauthPresets[key] = fn
}

// CopyHeaders copies ingress headers onto an outbound request.
func CopyHeaders(dst *http.Request, src http.Header) {
	for k, vals := range src {
		for _, v := range vals {
			dst.Header.Add(k, v)
		}
	}
}

// StripClient removes client credentials before upstream forward.
func StripClient(r *http.Request) {
	r.Header.Del("Authorization")
	r.Header.Del("x-api-key")
	r.Header.Del("X-Api-Key")
}

// Apply writes upstream auth headers from material.
// API key presets: bearer (default), x-api-key, or any other header name.
func Apply(m store.Material, r *http.Request, preset string) {
	StripClient(r)
	preset = strings.ToLower(strings.TrimSpace(preset))
	switch m.Kind {
	case store.KindAPIKey:
		applyAPIKey(m.APIKey, r, preset)
	case store.KindOAuth:
		r.Header.Set("Authorization", "Bearer "+strings.TrimSpace(m.AccessToken))
		applyOAuthPreset(preset, r)
	}
}

func applyAPIKey(key string, r *http.Request, preset string) {
	key = strings.TrimSpace(key)
	switch preset {
	case "", "bearer":
		r.Header.Set("Authorization", "Bearer "+key)
	case "x-api-key":
		r.Header.Set("x-api-key", key)
	default:
		r.Header.Set(preset, key)
	}
}

func applyOAuthPreset(preset string, r *http.Request) {
	if preset == "" {
		return
	}
	oauthPresetMu.RLock()
	fn := oauthPresets[preset]
	oauthPresetMu.RUnlock()
	if fn != nil {
		fn(r)
	}
}