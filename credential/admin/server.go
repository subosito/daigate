package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/ingress/adminauth"
	"github.com/subosito/daigate/ingress/keyring"
	"github.com/subosito/daigate/internal/limits"
)

const (
	snapshotStreamPoll      = 500 * time.Millisecond
	snapshotStreamKeepalive = 30 * time.Second
)

// Server is credential admin HTTP API.
type Server struct {
	Store           store.Store
	KeyStore        keyring.KeyStore
	Auth            *adminauth.Middleware
	KeyPolicy       MintPolicy
	ProvisionPolicy MintPolicy
	OnImport        func(profile, key string) (int64, error)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/healthz", s.healthz)
	protected := s.Auth.Require(http.HandlerFunc(s.route))
	mux.Handle("/v1/", protected)
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/v1/credentials" && r.Method == http.MethodGet:
		s.listCredentials(w, r)
	case path == "/v1/credentials" && r.Method == http.MethodPost:
		s.importCredential(w, r)
	case strings.HasPrefix(path, "/v1/credentials/") && r.Method == http.MethodGet:
		s.getCredential(w, r)
	case path == "/v1/snapshot/stream" && r.Method == http.MethodGet:
		s.snapshotStream(w, r)
	case path == "/v1/snapshot" && r.Method == http.MethodGet:
		s.snapshot(w, r)
	case path == "/v1/keys" && r.Method == http.MethodPost:
		s.createKey(w, r)
	case strings.HasPrefix(path, "/v1/keys/") && r.Method == http.MethodDelete:
		s.revokeKey(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) listCredentials(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSummaries(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(list)
}

func (s *Server) getCredential(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/v1/credentials/"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	cs, err := s.Store.GetSummary(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cs)
}

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	meta, err := s.Store.SnapshotMeta(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}

func (s *Server) snapshotStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	meta, err := s.Store.SnapshotMeta(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	lastGen := meta.Generation
	if err := writeSnapshotEvent(w, flusher, meta); err != nil {
		return
	}

	poll := time.NewTicker(snapshotStreamPoll)
	defer poll.Stop()
	keepalive := time.NewTicker(snapshotStreamKeepalive)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-poll.C:
			meta, err := s.Store.SnapshotMeta(ctx)
			if err != nil {
				return
			}
			if meta.Generation == lastGen {
				continue
			}
			if err := writeSnapshotEvent(w, flusher, meta); err != nil {
				return
			}
			lastGen = meta.Generation
		}
	}
}

func writeSnapshotEvent(w http.ResponseWriter, flusher http.Flusher, meta store.SnapshotMeta) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", b); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *Server) importCredential(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limits.MaxRequestBody))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var req struct {
		Provider   string          `json:"provider"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var cred struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	_ = json.Unmarshal(req.Credential, &cred)
	if cred.Type != "api_key" || cred.Key == "" {
		http.Error(w, "only api_key import supported", http.StatusBadRequest)
		return
	}
	var id int64
	if s.OnImport != nil {
		id, err = s.OnImport(req.Provider, cred.Key)
	} else {
		id, err = s.Store.PutAPIKey(r.Context(), req.Provider, cred.Key)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "provider": req.Provider})
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	role, _ := adminauth.RoleFrom(r.Context())
	policy := s.KeyPolicy
	if policy.MaxTTL == 0 {
		policy = KeysPolicyFromConfig(nil)
	}
	if role == adminauth.RoleProvision {
		policy = s.ProvisionPolicy
		if policy.MaxTTL == 0 {
			policy = DefaultProvisionPolicy()
		}
	}
	var req struct {
		Name   string   `json:"name"`
		Static bool     `json:"static"`
		TTL    string   `json:"ttl"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limits.MaxRequestBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if role == adminauth.RoleProvision && req.Static {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	kind := keyring.KindIssued
	ttl := 24 * time.Hour
	if req.Static {
		kind = keyring.KindStatic
		ttl = 0
	} else {
		var err error
		ttl, err = ParseDuration(req.TTL, 24*time.Hour, policy.MaxTTL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	requestedScopes := req.Scopes
	if role == adminauth.RoleProvision {
		requestedScopes = nil
	}
	scopes, err := policy.ResolveScopes(requestedScopes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	secret, id, err := s.KeyStore.Create(r.Context(), req.Name, kind, ttl, scopes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "key": secret})
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/v1/keys/"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.KeyStore.Revoke(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}