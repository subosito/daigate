package admin_test

import (
	"testing"
	"time"

	"github.com/subosito/daigate/credential/admin"
	"github.com/subosito/daigate/internal/config"
)

func TestProvisionPolicyDefaults(t *testing.T) {
	p := admin.ProvisionPolicyFromConfig(nil)
	if p.MaxTTL != 24*time.Hour {
		t.Fatalf("max_ttl=%s", p.MaxTTL)
	}
	if len(p.Scopes) == 0 {
		t.Fatal("expected default provision scopes")
	}
	scopes, err := p.ResolveScopes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) == 0 {
		t.Fatal("expected scoped provision key")
	}
}

func TestKeysPolicyAllowlist(t *testing.T) {
	f := &config.File{}
	f.Admin.Keys.Scopes = []string{"model:foo"}
	p := admin.KeysPolicyFromConfig(f)
	_, err := p.ResolveScopes([]string{"model:bar"})
	if err == nil {
		t.Fatal("expected scope rejection")
	}
	scopes, err := p.ResolveScopes([]string{"model:foo"})
	if err != nil || len(scopes) != 1 {
		t.Fatalf("scopes=%v err=%v", scopes, err)
	}
}

func TestParseDurationCapsMax(t *testing.T) {
	d, err := admin.ParseDuration("48h", time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if d != 24*time.Hour {
		t.Fatalf("got %s", d)
	}
}