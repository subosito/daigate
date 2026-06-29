package adminauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/subosito/daigate/credential/seal"
	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/ingress/adminauth"
	"github.com/subosito/daigate/internal/config"
)

func TestLoadDisabledAdminPlane(t *testing.T) {
	f := &config.File{}
	disabled := false
	f.Admin.Enable = &disabled
	mw, err := adminauth.Load(nil, f)
	if err != nil {
		t.Fatal(err)
	}
	if mw != nil {
		t.Fatal("expected nil middleware when admin disabled")
	}
}

func TestLoadDBToken(t *testing.T) {
	key, _ := seal.ParseKey("GGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG=")
	path := filepath.Join(t.TempDir(), "broker.db")
	st, err := store.OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db, err := store.SQLDB(st)
	if err != nil {
		t.Fatal(err)
	}
	ts := adminauth.NewSQLTokenStore(db)
	const dbTok = "db-only-admin-token-value"
	if err := ts.Insert(context.Background(), "test", adminauth.RoleAdmin, dbTok); err != nil {
		t.Fatal(err)
	}

	f := &config.File{}
	f.Admin.Listen = "127.0.0.1:9421"
	f.Admin.Tokens.AdminEnv = "DAIGATE_ADMIN_ENV_TEST"
	f.Admin.Tokens.ProvisionEnv = "DAIGATE_PROVISION_ENV_TEST"
	t.Setenv("DAIGATE_ADMIN_ENV_TEST", "env-admin-token")
	t.Setenv("DAIGATE_PROVISION_ENV_TEST", "env-provision-token")

	mw, err := adminauth.Load(db, f)
	if err != nil {
		t.Fatal(err)
	}
	h := mw.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	tsrv := httptest.NewServer(h)
	defer tsrv.Close()

	req, _ := http.NewRequest(http.MethodGet, tsrv.URL, nil)
	req.Header.Set("Authorization", "Bearer "+dbTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("db token status=%d", resp.StatusCode)
	}
}