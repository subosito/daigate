package store_test

import (
	"testing"

	"github.com/subosito/daigate/credential/store"
)

func TestMaterialFromDecryptedOAuthExtras(t *testing.T) {
	raw := []byte(`{"type":"oauth","access":"tok","refresh":"ref","accountId":"acct-1","project_id":"proj-2"}`)
	mat, err := store.MaterialFromDecrypted("p", store.KindOAuth, raw)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Extra("account_id") != "acct-1" {
		t.Fatalf("account_id=%q", mat.Extra("account_id"))
	}
	if mat.Extra("project_id") != "proj-2" {
		t.Fatalf("project_id=%q", mat.Extra("project_id"))
	}
}

func TestMaterialFromDecryptedOAuthExtrasFieldWins(t *testing.T) {
	raw := []byte(`{"type":"oauth","access":"tok","extras":{"account_id":"from-extras"},"accountId":"legacy"}`)
	mat, err := store.MaterialFromDecrypted("p", store.KindOAuth, raw)
	if err != nil {
		t.Fatal(err)
	}
	if mat.Extra("account_id") != "from-extras" {
		t.Fatalf("account_id=%q", mat.Extra("account_id"))
	}
}