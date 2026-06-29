package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCredentialLoginCLI(t *testing.T) {
	bin := buildCLIBinary(t)
	srv := newCLIOAuthServer(t)
	defer srv.Close()

	work := t.TempDir()
	cfgPath := writeOAuthCLIConfig(t, work, srv.URL)
	vaultKey := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

	t.Run("manual", func(t *testing.T) {
		out, err := runLoginCLI(t, bin, cfgPath, vaultKey, work, "manual", "auth-code-1\n", nil)
		if err != nil {
			t.Fatalf("err=%v out=%s", err, out)
		}
		assertLoginOutput(t, out)
	})

	t.Run("device", func(t *testing.T) {
		go func() {
			time.Sleep(200 * time.Millisecond)
			srv.approveDevice()
		}()
		out, err := runLoginCLI(t, bin, cfgPath, vaultKey, work, "device", "", nil)
		if err != nil {
			t.Fatalf("err=%v out=%s", err, out)
		}
		assertLoginOutput(t, out)
	})

	t.Run("browser", func(t *testing.T) {
		out, err := runBrowserLoginCLI(t, bin, cfgPath, vaultKey, work)
		if err != nil {
			t.Fatalf("err=%v out=%s", err, out)
		}
		assertLoginOutput(t, out)
	})

	t.Run("auto", func(t *testing.T) {
		go func() {
			time.Sleep(200 * time.Millisecond)
			srv.approveDevice()
		}()
		out, err := runLoginCLI(t, bin, cfgPath, vaultKey, work, "auto", "", []string{"DAIGATE_FORCE_DEVICE=1"})
		if err != nil {
			t.Fatalf("err=%v out=%s", err, out)
		}
		assertLoginOutput(t, out)
	})
}

func assertLoginOutput(t *testing.T, out string) {
	t.Helper()
	if !strings.Contains(out, "logged in id=") || !strings.Contains(out, "profile=test-oauth") {
		t.Fatalf("unexpected output: %s", out)
	}
	for _, secret := range []string{"access-xyz", "refresh-xyz", "sk-"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secrets leaked in CLI output: %s", out)
		}
	}
}

func buildCLIBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "daigate")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/daigate")
	cmd.Dir = moduleRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}

func writeOAuthCLIConfig(t *testing.T, work, oauthBase string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	adminAddr := ln.Addr().String()
	_ = ln.Close()
	cfg := fmt.Sprintf(`serve:
  data_listen: "127.0.0.1:0"
  catalog: providers.yaml
admin:
  listen: "%s"
credential:
  broker: broker.db
ingress:
  client_auth: keyring
adapters:
  enable: [passthrough]
credential_profiles:
  test-oauth:
    kind: oauth
    oauth:
      authorize_url: %s/oauth/authorize
      token_url: %s/oauth/token
      device_authorize_url: %s/oauth/device/code
      client_id: test-client
      pkce: true
`, adminAddr, oauthBase, oauthBase, oauthBase)
	path := filepath.Join(work, "daigate.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runLoginCLI(t *testing.T, bin, cfgPath, vaultKey, work, flow, stdin string, extraEnv []string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "credential", "login", "test-oauth", "--flow="+flow, "--config", cfgPath)
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "DAIGATE_BROKER_KEY="+vaultKey)
	cmd.Env = append(cmd.Env, extraEnv...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runBrowserLoginCLI(t *testing.T, bin, cfgPath, vaultKey, work string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "credential", "login", "test-oauth", "--flow=browser", "--config", cfgPath)
	cmd.Dir = work
	cmd.Env = append(os.Environ(), "DAIGATE_BROKER_KEY="+vaultKey)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	go func() {
		state, callbackHost := readOAuthCallback(stderr)
		if state == "" || callbackHost == "" {
			return
		}
		time.Sleep(100 * time.Millisecond)
		redirect := fmt.Sprintf("http://%s/oauth/callback?code=auth-code-1&state=%s", callbackHost, url.QueryEscape(state))
		req, _ := http.NewRequest(http.MethodGet, redirect, nil)
		_, _ = http.DefaultClient.Do(req)
	}()

	var buf strings.Builder
	_, _ = io.Copy(&buf, stdout)
	err = cmd.Wait()
	return buf.String(), err
}

func readOAuthCallback(r io.Reader) (state, callbackHost string) {
	sc := bufio.NewScanner(r)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, "Open:") {
			continue
		}
		authURL := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Open:"))
		u, err := url.Parse(authURL)
		if err != nil {
			continue
		}
		state = u.Query().Get("state")
		redir := u.Query().Get("redirect_uri")
		if redir == "" {
			continue
		}
		ru, err := url.Parse(redir)
		if err != nil {
			continue
		}
		return state, ru.Host
	}
	return "", ""
}

type cliOAuthSrv struct {
	*httptest.Server
	mu       sync.Mutex
	approved bool
}

func newCLIOAuthServer(t *testing.T) *cliOAuthSrv {
	t.Helper()
	o := &cliOAuthSrv{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "device-code-123", "user_code": "ABCD-1234",
			"verification_uri": o.URL + "/verify", "expires_in": 300, "interval": 1,
		})
	})
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "urn:ietf:params:oauth:grant-type:device_code":
			o.mu.Lock()
			ok := o.approved
			o.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
				return
			}
		case "authorization_code":
			if r.Form.Get("code_verifier") == "" {
				http.Error(w, "missing verifier", http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-xyz", "refresh_token": "refresh-xyz", "expires_in": 3600,
		})
	})
	o.Server = httptest.NewServer(mux)
	return o
}

func (o *cliOAuthSrv) approveDevice() {
	o.mu.Lock()
	o.approved = true
	o.mu.Unlock()
}

func parseAdminListen(yaml string) string {
	for _, line := range strings.Split(yaml, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "listen:") {
			return strings.Trim(strings.TrimPrefix(line, "listen:"), `" `)
		}
	}
	return "127.0.0.1:9421"
}