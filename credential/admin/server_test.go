package admin_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/subosito/daigate/credential/admin"
	"github.com/subosito/daigate/credential/seal"
	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/ingress/adminauth"
	"github.com/subosito/daigate/ingress/keyring"
)

func TestListCredentialsNoSecrets(t *testing.T) {
	key, _ := seal.ParseKey("DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD=")
	st := store.NewMemory(key)
	_, _ = st.PutAPIKey(t.Context(), "p", "sk-leaked-should-not-appear")
	ks := keyring.NewMemoryStore()
	auth, err := adminauth.NewFromPlain("admin-tok", "prov-tok")
	if err != nil {
		t.Fatal(err)
	}

	srv := &admin.Server{Store: st, KeyStore: ks, Auth: auth}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer admin-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	s := string(body)
	if strings.Contains(s, "sk-leaked") {
		t.Fatalf("response leaked api key: %s", s)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		for _, field := range []string{"access", "refresh", "key", "api_key", "access_token", "refresh_token"} {
			if _, ok := row[field]; ok {
				t.Fatalf("response contains secret field %q: %v", field, row)
			}
		}
	}
}

func TestSnapshotStreamGenerationBump(t *testing.T) {
	key, _ := seal.ParseKey("GGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG=")
	st := store.NewMemory(key)
	ks := keyring.NewMemoryStore()
	auth, err := adminauth.NewFromPlain("admin-tok", "prov-tok")
	if err != nil {
		t.Fatal(err)
	}
	srv := &admin.Server{Store: st, KeyStore: ks, Auth: auth}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/snapshot/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer admin-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%q", ct)
	}

	events := make(chan store.SnapshotMeta, 2)
	errCh := make(chan error, 1)
	go func() {
		br := bufio.NewReader(resp.Body)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				if err == io.EOF || ctx.Err() != nil {
					return
				}
				errCh <- err
				return
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var meta store.SnapshotMeta
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &meta); err != nil {
				errCh <- err
				return
			}
			select {
			case events <- meta:
			default:
			}
		}
	}()

	select {
	case meta := <-events:
		if meta.Generation != 1 {
			t.Fatalf("initial generation=%d", meta.Generation)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initial snapshot event")
	}

	if _, err := st.PutAPIKey(t.Context(), "p", "sk-stream"); err != nil {
		t.Fatal(err)
	}

	select {
	case meta := <-events:
		if meta.Generation != 2 {
			t.Fatalf("bumped generation=%d", meta.Generation)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for bumped snapshot event")
	}
	cancel()
}

func TestProvisionDeniedSnapshotStream(t *testing.T) {
	key, _ := seal.ParseKey("IIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIIII=")
	st := store.NewMemory(key)
	ks := keyring.NewMemoryStore()
	auth, err := adminauth.NewFromPlain("admin-tok", "prov-tok")
	if err != nil {
		t.Fatal(err)
	}
	srv := &admin.Server{Store: st, KeyStore: ks, Auth: auth}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/snapshot/stream", nil)
	req.Header.Set("Authorization", "Bearer prov-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestProvisionDeniedCredentials(t *testing.T) {
	key, _ := seal.ParseKey("EEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE=")
	st := store.NewMemory(key)
	ks := keyring.NewMemoryStore()
	auth, err := adminauth.NewFromPlain("admin-tok", "prov-tok")
	if err != nil {
		t.Fatal(err)
	}
	srv := &admin.Server{Store: st, KeyStore: ks, Auth: auth}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer prov-tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestImportCredentialPluralPath(t *testing.T) {
	key, _ := seal.ParseKey("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF=")
	st := store.NewMemory(key)
	ks := keyring.NewMemoryStore()
	auth, err := adminauth.NewFromPlain("admin-tok", "prov-tok")
	if err != nil {
		t.Fatal(err)
	}
	srv := &admin.Server{Store: st, KeyStore: ks, Auth: auth}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"provider":"p","credential":{"type":"api_key","key":"sk-import"}}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/credentials", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer admin-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
}

func TestProvisionMintsKeyOnAdminListener(t *testing.T) {
	key, _ := seal.ParseKey("HHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHHH=")
	st := store.NewMemory(key)
	ks := keyring.NewMemoryStore()
	auth, err := adminauth.NewFromPlain("admin-tok", "prov-tok")
	if err != nil {
		t.Fatal(err)
	}
	srv := &admin.Server{
		Store:           st,
		KeyStore:        ks,
		Auth:            auth,
		ProvisionPolicy: admin.DefaultProvisionPolicy(),
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/keys", strings.NewReader(`{"name":"ci"}`))
	req.Header.Set("Authorization", "Bearer prov-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	keyStr, _ := out["key"].(string)
	if !strings.HasPrefix(keyStr, "sk-dg-") {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestProvisionCannotMintStaticKey(t *testing.T) {
	ks := keyring.NewMemoryStore()
	auth, err := adminauth.NewFromPlain("admin-tok", "prov-tok")
	if err != nil {
		t.Fatal(err)
	}
	srv := &admin.Server{KeyStore: ks, Auth: auth, ProvisionPolicy: admin.DefaultProvisionPolicy()}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/keys", strings.NewReader(`{"name":"ci","static":true}`))
	req.Header.Set("Authorization", "Bearer prov-tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}