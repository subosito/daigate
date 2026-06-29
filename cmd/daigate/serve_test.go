package main

import (
	"testing"

	"github.com/subosito/daigate/gateway"
	"github.com/subosito/daigate/ingress/adminauth"
	"github.com/subosito/daigate/internal/config"
)

func TestServeConfigDataOnlyWithoutAdminTokens(t *testing.T) {
	f := &config.File{}
	disabled := false
	f.Admin.Enable = &disabled
	mw, err := adminauth.Load(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	if mw != nil {
		t.Fatal("expected nil admin middleware when admin disabled")
	}
	_, err = gateway.New(gateway.Config{
		AdminEnabled: false,
		AdminAuth:    nil,
	})
	if err == nil {
		t.Fatal("expected catalog required error without full config")
	}
}