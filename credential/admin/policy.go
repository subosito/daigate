package admin

import (
	"fmt"
	"strings"
	"time"

	"github.com/subosito/daigate/internal/config"
)

// MintPolicy caps gateway key minting.
type MintPolicy struct {
	MaxTTL           time.Duration
	Scopes           []string // provision: fixed scopes; admin keys: allowlist (empty = unrestricted)
	RequireAllowlist bool     // when true, requested scopes must match allowlist
}

// DefaultProvisionPolicy is used when config omits admin.provision.
func DefaultProvisionPolicy() MintPolicy {
	return MintPolicy{
		MaxTTL: 24 * time.Hour,
		Scopes: []string{
			"wire:openai-chat-completions",
			"wire:anthropic-messages",
			"wire:openai-responses",
		},
		RequireAllowlist: true,
	}
}

// ParseDuration parses a ttl string with fallback and max cap.
func ParseDuration(raw string, fallback, max time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid ttl %q: %w", raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("ttl must be positive")
	}
	if max > 0 && d > max {
		return max, nil
	}
	return d, nil
}

// ResolveScopes returns scopes to store for a mint request.
func (p MintPolicy) ResolveScopes(requested []string) ([]string, error) {
	if p.RequireAllowlist {
		if len(p.Scopes) == 0 {
			return nil, fmt.Errorf("provision scopes not configured")
		}
		return append([]string(nil), p.Scopes...), nil
	}
	if len(p.Scopes) == 0 {
		return requested, nil
	}
	if len(requested) == 0 {
		return nil, fmt.Errorf("scopes required")
	}
	for _, s := range requested {
		if !scopeAllowed(s, p.Scopes) {
			return nil, fmt.Errorf("scope %q not allowed", s)
		}
	}
	return requested, nil
}

// ProvisionPolicyFromConfig builds provision mint caps from daigate.yaml.
func ProvisionPolicyFromConfig(f *config.File) MintPolicy {
	p := DefaultProvisionPolicy()
	if f == nil {
		return p
	}
	cfg := f.Admin.Provision
	if cfg.MaxTTL != "" {
		if d, err := time.ParseDuration(cfg.MaxTTL); err == nil && d > 0 {
			p.MaxTTL = d
		}
	}
	if len(cfg.Scopes) > 0 {
		p.Scopes = append([]string(nil), cfg.Scopes...)
	}
	return p
}

// KeysPolicyFromConfig builds admin POST /v1/keys caps from daigate.yaml.
func KeysPolicyFromConfig(f *config.File) MintPolicy {
	p := MintPolicy{MaxTTL: 720 * time.Hour}
	if f == nil {
		return p
	}
	cfg := f.Admin.Keys
	if cfg.MaxTTL != "" {
		if d, err := time.ParseDuration(cfg.MaxTTL); err == nil && d > 0 {
			p.MaxTTL = d
		}
	}
	if len(cfg.Scopes) > 0 {
		p.Scopes = append([]string(nil), cfg.Scopes...)
	}
	return p
}

func scopeAllowed(scope string, allowlist []string) bool {
	scope = strings.TrimSpace(scope)
	for _, a := range allowlist {
		a = strings.TrimSpace(a)
		if a == "*" || a == scope {
			return true
		}
	}
	return false
}