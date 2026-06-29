package gateway

import (
	"database/sql"
	"net/http"
	"sync"

	"github.com/subosito/daigate/ingress/keyring"
	"github.com/subosito/daigate/internal/config"
)

// AdminIssuerMount registers admin-plane routes for a gateway key issuer driver.
type AdminIssuerMount func(mux *http.ServeMux, db *sql.DB, ks keyring.KeyStore, entry config.IssuerEntry) error

var (
	issuerMu     sync.RWMutex
	issuerMounts = map[string]AdminIssuerMount{}
)

// RegisterAdminIssuer registers an ingress issuer driver (name from ingress.issuers[].driver).
func RegisterAdminIssuer(driver string, mount AdminIssuerMount) {
	issuerMu.Lock()
	defer issuerMu.Unlock()
	issuerMounts[driver] = mount
}

func mountAdminIssuers(mux *http.ServeMux, db *sql.DB, ks keyring.KeyStore, issuers []config.IssuerEntry) {
	issuerMu.RLock()
	defer issuerMu.RUnlock()
	for _, entry := range issuers {
		mount := issuerMounts[entry.Driver]
		if mount == nil {
			continue
		}
		_ = mount(mux, db, ks, entry)
	}
}