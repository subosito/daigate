package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/subosito/daigate/adaptersdk"
	"github.com/subosito/daigate/catalog"
	"github.com/subosito/daigate/credential/admin"
	"github.com/subosito/daigate/credential/seal"
	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/ingress/adminauth"
	"github.com/subosito/daigate/ingress/keyring"
	"github.com/subosito/daigate/internal/config"
	"github.com/subosito/daigate/upstream"
	"github.com/subosito/daigate/wire"
)

// DataMount registers extra data-plane routes before the wire engine catch-all.
type DataMount func(mux *http.ServeMux, engine *wire.Engine)

// Config assembles a running gateway (data + admin listeners, shared credential store).
type Config struct {
	ConfigFile  string
	Config      *config.File
	Catalog     *catalog.Catalog
	Store       store.Store
	KeyStore    keyring.KeyStore
	Adapters    *adaptersdk.Registry
	AdminAuth     *adminauth.Middleware
	AdminEnabled  bool
	DataListen    string
	AdminListen   string
	DataMount        DataMount
	WrapDataHandler  func(http.Handler) http.Handler
}

// Gateway holds HTTP servers and lifecycle state.
type Gateway struct {
	cfg        Config
	dataSrv    *http.Server
	adminSrv   *http.Server
	dataLn     net.Listener
	adminLn    net.Listener
	lnMu       sync.RWMutex
	active     sync.WaitGroup
	shutdownMu sync.Mutex
	draining   bool
}

// DataAddr returns bound data listener address (empty until ListenAndServe).
func (g *Gateway) DataAddr() string {
	g.lnMu.RLock()
	defer g.lnMu.RUnlock()
	if g.dataLn == nil {
		return ""
	}
	return g.dataLn.Addr().String()
}

// New builds a gateway from config.
func New(cfg Config) (*Gateway, error) {
	if cfg.Catalog == nil {
		return nil, fmt.Errorf("catalog required")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("store required")
	}
	if cfg.KeyStore == nil {
		return nil, fmt.Errorf("keystore required")
	}
	if cfg.Adapters == nil {
		return nil, fmt.Errorf("adapters registry required")
	}
	if cfg.AdminEnabled && cfg.AdminAuth == nil {
		return nil, fmt.Errorf("admin auth required when admin plane enabled")
	}
	return &Gateway{cfg: cfg}, nil
}

// OpenStore opens credential store from config (sqlite default; extension backends via registry).
func OpenStore(f *config.File) (store.Store, keyring.KeyStore, error) {
	keyB64, err := config.BrokerKey(f)
	if err != nil {
		return nil, nil, err
	}
	key, err := seal.ParseKey(keyB64)
	if err != nil {
		return nil, nil, err
	}
	sqlite, err := store.OpenSQLite(f.Credential.Broker, key)
	if err != nil {
		return nil, nil, err
	}
	ks := keyring.NewSQLStore(sqlite.DB())

	backend := strings.TrimSpace(f.Credential.Backend)
	if backend == "" || backend == "sqlite" {
		return sqlite, ks, nil
	}
	st, err := openCredentialBackend(backend, f, sqlite)
	if err != nil {
		sqlite.Close()
		return nil, nil, err
	}
	return st, ks, nil
}

func (g *Gateway) track(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.shutdownMu.Lock()
		if g.draining {
			g.shutdownMu.Unlock()
			http.Error(w, "shutting down", http.StatusServiceUnavailable)
			return
		}
		g.active.Add(1)
		g.shutdownMu.Unlock()
		defer g.active.Done()
		next.ServeHTTP(w, r)
	})
}

func (g *Gateway) dataHandler() http.Handler {
	engine := &wire.Engine{
		Catalog: g.cfg.Catalog,
		Store:   g.cfg.Store,
		Adapters: g.cfg.Adapters,
		Auth:    &keyring.Authenticator{Store: g.cfg.KeyStore},
		Client:  upstream.NewClient(),
	}
	mux := http.NewServeMux()
	if g.cfg.DataMount != nil {
		g.cfg.DataMount(mux, engine)
	}
	mux.Handle("/", g.track(engine.Handler()))
	h := http.Handler(mux)
	if g.cfg.WrapDataHandler != nil {
		h = g.cfg.WrapDataHandler(h)
	}
	return h
}

func (g *Gateway) adminHandler() http.Handler {
	srv := &admin.Server{
		Store:           g.cfg.Store,
		KeyStore:        g.cfg.KeyStore,
		Auth:            g.cfg.AdminAuth,
		KeyPolicy:       admin.KeysPolicyFromConfig(g.cfg.Config),
		ProvisionPolicy: admin.ProvisionPolicyFromConfig(g.cfg.Config),
	}
	mux := http.NewServeMux()
	if db, err := store.BrokerDB(g.cfg.Store); err == nil {
		mountAdminIssuers(mux, db, g.cfg.KeyStore, g.cfg.Config.Ingress.Issuers)
	}
	mux.Handle("/", g.track(srv.Handler()))
	return mux
}

// ListenAndServe starts data and admin listeners until signal or error.
// Observability: standalone binaries call observability.Boot before this; library embedders call observability.Hook.
func (g *Gateway) ListenAndServe(ctx context.Context) error {
	g.dataSrv = &http.Server{Addr: g.cfg.DataListen, Handler: g.dataHandler()}
	if g.cfg.AdminEnabled {
		g.adminSrv = &http.Server{Addr: g.cfg.AdminListen, Handler: g.adminHandler()}
	}

	errCh := make(chan error, 2)
	go func() {
		ln, err := net.Listen("tcp", g.cfg.DataListen)
		if err != nil {
			errCh <- fmt.Errorf("data listen: %w", err)
			return
		}
		g.lnMu.Lock()
		g.dataLn = ln
		g.lnMu.Unlock()
		slog.Info("gateway listening", "plane", "data", "addr", ln.Addr().String())
		errCh <- g.dataSrv.Serve(ln)
	}()
	if g.cfg.AdminEnabled {
		go func() {
			ln, err := net.Listen("tcp", g.cfg.AdminListen)
			if err != nil {
				errCh <- fmt.Errorf("admin listen: %w", err)
				return
			}
			g.lnMu.Lock()
			g.adminLn = ln
			g.lnMu.Unlock()
			slog.Info("gateway listening", "plane", "admin", "addr", ln.Addr().String())
			errCh <- g.adminSrv.Serve(ln)
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		return g.Shutdown(context.Background())
	case sig := <-sigCh:
		slog.Info("gateway drain", "signal", sig.String())
		return g.Shutdown(context.Background())
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		_ = g.Shutdown(context.Background())
		return err
	}
}

// Shutdown stops accepts and drains in-flight requests.
func (g *Gateway) Shutdown(ctx context.Context) error {
	g.shutdownMu.Lock()
	g.draining = true
	g.shutdownMu.Unlock()

	slog.Info("gateway drain", "phase", "stop_accept")
	shutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	done := make(chan struct{})
	go func() {
		g.active.Wait()
		close(done)
	}()

	if g.dataSrv != nil {
		_ = g.dataSrv.Shutdown(shutCtx)
	}
	if g.adminSrv != nil {
		_ = g.adminSrv.Shutdown(shutCtx)
	}

	select {
	case <-done:
		slog.Info("gateway drain", "phase", "complete")
	case <-shutCtx.Done():
		slog.Warn("gateway drain", "phase", "timeout")
	}
	return nil
}

// ActiveWaitGroup exposes in-flight tracking for tests.
func (g *Gateway) ActiveWaitGroup() *sync.WaitGroup { return &g.active }