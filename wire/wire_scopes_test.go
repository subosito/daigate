package wire_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subosito/daigate/adaptersdk"
	"github.com/subosito/daigate/passthrough"
	"github.com/subosito/daigate/catalog"
	"github.com/subosito/daigate/credential/seal"
	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/ingress/keyring"
	"github.com/subosito/daigate/upstream"
	"github.com/subosito/daigate/wire"
)

func TestModelsListFilteredByScope(t *testing.T) {
	engine, secret := testModelsEngineWithKeyScoped(t, []string{"model:mock-chat"})
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "mock-chat") {
		t.Fatalf("body=%s", body)
	}

	engine2, secret2 := testModelsEngineWithKeyScoped(t, []string{"model:other"})
	ts2 := httptest.NewServer(engine2.Handler())
	defer ts2.Close()
	req2, _ := http.NewRequest(http.MethodGet, ts2.URL+"/v1/models", nil)
	req2.Header.Set("Authorization", "Bearer "+secret2)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if strings.Contains(string(body2), "mock-chat") {
		t.Fatalf("expected filtered out: %s", body2)
	}
}

func testModelsEngineWithKeyScoped(t *testing.T, scopes []string) (*wire.Engine, string) {
	t.Helper()
	p := filepath.Join("..", "testdata", "fixtures", "providers.yaml")
	cat, err := catalog.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := seal.ParseKey("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF=")
	st := store.NewMemory(key)
	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, scopes)
	return &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth:   &keyring.Authenticator{Store: ks},
		Client: upstream.NewClient(),
	}, secret
}