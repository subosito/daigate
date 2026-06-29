package wire_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subosito/daigate/adaptersdk"
	"github.com/subosito/daigate/passthrough"
	"github.com/subosito/daigate/catalog"
	"github.com/subosito/daigate/credential/seal"
	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/ingress/keyring"
	"github.com/subosito/daigate/observability"
	"github.com/subosito/daigate/upstream"
	"github.com/subosito/daigate/wire"
)

func testEngine(t *testing.T) (*wire.Engine, string) {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	t.Cleanup(up.Close)

	providers := `
providers:
  mock:
    credential_profile: mock
    protocol: openai-chat-completions
    base_url: ` + up.URL + `
models:
  mock-chat:
    modalities:
      chat:
        wire: openai-chat-completions
        providers:
          - provider_ref: mock
            model: echo
`
	p := filepath.Join(t.TempDir(), "providers.yaml")
	if err := os.WriteFile(p, []byte(providers), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := seal.ParseKey("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF=")
	st := store.NewMemory(key)
	_, _ = st.PutAPIKey(t.Context(), "mock", "sk-up")
	reg := adaptersdk.NewRegistry()
	_ = passthrough.New().Register(reg)
	ks := keyring.NewMemoryStore()
	secret, _, _ := ks.Create(t.Context(), "client", keyring.KindStatic, 0, nil)
	return &wire.Engine{
		Catalog: cat, Store: st, Adapters: reg,
		Auth: &keyring.Authenticator{Store: ks}, Client: upstream.NewClient(),
	}, secret
}

func captureObservability(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	observability.SetTestLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { observability.SetTestLogger(slog.Default()) })
	return &buf
}

func TestHealthzEmitsIngressLog(t *testing.T) {
	engine, _ := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, `"wire":"healthz"`) && !strings.Contains(out, "healthz") {
		t.Fatalf("missing healthz ingress log: %s", out)
	}
}

func TestUnauthorizedEmitsIngressLog(t *testing.T) {
	engine, _ := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(`{"model":"mock-chat"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, `"status":401`) && !strings.Contains(out, "status") {
		t.Fatalf("missing 401 ingress log: %s", out)
	}
}

func TestNotFoundEmitsIngressLog(t *testing.T) {
	engine, _ := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/unknown-route")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, "/v1/unknown-route") && !strings.Contains(out, `"status":404`) {
		t.Fatalf("missing 404 ingress log: %s", out)
	}
}

func TestBadModelEmitsIngressLog(t *testing.T) {
	engine, secret := testEngine(t)
	buf := captureObservability(t)
	ts := httptest.NewServer(engine.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", strings.NewReader(`{"model":"unknown-model"}`))
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, `"status":400`) && !strings.Contains(out, "status") {
		t.Fatalf("missing 400 ingress log: %s", out)
	}
}