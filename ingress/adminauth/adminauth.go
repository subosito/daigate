package adminauth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/subosito/daigate/ingress/argonhash"
	"github.com/subosito/daigate/internal/config"
)

// Role is control-plane auth role.
type Role string

const (
	RoleAdmin     Role = "admin"
	RoleProvision Role = "provision"
)

var provisionDeny = map[string]bool{
	"/v1/credentials": true,
	"/v1/snapshot":    true,
}

type tokenEntry struct {
	role   Role
	sealed string
}

// Middleware enforces admin or provision bearer tokens.
type Middleware struct {
	entries []tokenEntry
}

// NewFromPlain builds middleware from explicit tokens (tests).
func NewFromPlain(admin, provision string) (*Middleware, error) {
	var entries []tokenEntry
	if admin != "" {
		sealed, err := argonhash.Seal(admin)
		if err != nil {
			return nil, err
		}
		entries = append(entries, tokenEntry{role: RoleAdmin, sealed: sealed})
	}
	if provision != "" {
		sealed, err := argonhash.Seal(provision)
		if err != nil {
			return nil, err
		}
		entries = append(entries, tokenEntry{role: RoleProvision, sealed: sealed})
	}
	if len(entries) == 0 {
		return nil, errors.New("at least one admin token required")
	}
	return &Middleware{entries: entries}, nil
}

// Load builds middleware when the admin plane is enabled.
// Returns (nil, nil) when admin is disabled. When enabled, at least one token
// (env admin, env provision, or DB row) is required — not both env vars.
func Load(db *sql.DB, f *config.File) (*Middleware, error) {
	if !f.AdminPlaneEnabled() {
		return nil, nil
	}
	var entries []tokenEntry
	if tok := strings.TrimSpace(os.Getenv(f.Admin.Tokens.AdminEnv)); tok != "" {
		sealed, err := argonhash.Seal(tok)
		if err != nil {
			return nil, err
		}
		entries = append(entries, tokenEntry{role: RoleAdmin, sealed: sealed})
	}
	if tok := strings.TrimSpace(os.Getenv(f.Admin.Tokens.ProvisionEnv)); tok != "" {
		sealed, err := argonhash.Seal(tok)
		if err != nil {
			return nil, err
		}
		entries = append(entries, tokenEntry{role: RoleProvision, sealed: sealed})
	}
	if db != nil {
		dbEntries, err := loadDBTokens(db)
		if err != nil {
			return nil, err
		}
		entries = append(entries, dbEntries...)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("admin plane enabled: set %s or %s, or create admin token in broker.db",
			f.Admin.Tokens.AdminEnv, f.Admin.Tokens.ProvisionEnv)
	}
	return &Middleware{entries: entries}, nil
}

func loadDBTokens(db *sql.DB) ([]tokenEntry, error) {
	rows, err := db.Query(`SELECT role, hash FROM admin_tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []tokenEntry
	for rows.Next() {
		var role, sealed string
		if err := rows.Scan(&role, &sealed); err != nil {
			return nil, err
		}
		out = append(out, tokenEntry{role: Role(role), sealed: sealed})
	}
	return out, rows.Err()
}

func (m *Middleware) roleFor(tok string) (Role, bool) {
	for _, e := range m.entries {
		if argonhash.Verify(tok, e.sealed) {
			return e.role, true
		}
	}
	return "", false
}

func bearer(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", errors.New("unauthorized")
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")), nil
}

type ctxKey struct{}

// WithRole attaches role to context.
func WithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, ctxKey{}, role)
}

// RoleFrom returns role from context.
func RoleFrom(ctx context.Context) (Role, bool) {
	v, ok := ctx.Value(ctxKey{}).(Role)
	return v, ok
}

// Require wraps handler with auth.
func (m *Middleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, err := bearer(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		role, ok := m.roleFor(tok)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if role == RoleProvision {
			path := r.URL.Path
			for prefix := range provisionDeny {
				if strings.HasPrefix(path, prefix) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(WithRole(r.Context(), role)))
	})
}

// SQLTokenStore persists admin tokens in broker.db (loaded by serve on startup).
type SQLTokenStore struct {
	db *sql.DB
}

func NewSQLTokenStore(db *sql.DB) *SQLTokenStore { return &SQLTokenStore{db: db} }

func (s *SQLTokenStore) Insert(ctx context.Context, name string, role Role, plain string) error {
	sealed, err := argonhash.Seal(plain)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO admin_tokens (name, role, hash, created_at)
		VALUES (?, ?, ?, ?)`, name, string(role), sealed, now)
	return err
}