package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/subosito/daigate/adaptersdk"
	"github.com/subosito/daigate/catalog"
	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/ingress/adminauth"
	"github.com/subosito/daigate/internal/config"
	"github.com/subosito/daigate/observability"
)

// ServeOptions boots a standalone gateway from daigate.yaml.
type ServeOptions struct {
	ConfigPath  string
	ServiceName string
	Registry    func(cfg *ConfigFile) (*adaptersdk.Registry, error)
	CatalogLoad       func(path string) (*catalog.Catalog, error)
	DataMount         DataMount
	WrapDataHandler   func(http.Handler) http.Handler
}

// Serve loads config, opens stores, and runs until ctx is cancelled or error.
// Standalone binaries call Boot/ShutdownGraceful; library embedders use EmbedServe.
func Serve(ctx context.Context, opts ServeOptions) error {
	gw, err := openGateway(opts)
	if err != nil {
		return err
	}
	defer gw.cfg.Store.Close()

	name := serviceName(opts.ServiceName)
	observability.Boot(name)
	defer observability.ShutdownGraceful()
	return gw.ListenAndServe(ctx)
}

// EmbedServe runs the gateway when the host already owns OTel export.
// Call observability.Hook via this helper after host Boot — do not call Boot here.
func EmbedServe(ctx context.Context, opts ServeOptions) error {
	gw, err := openGateway(opts)
	if err != nil {
		return err
	}
	defer gw.cfg.Store.Close()

	observability.Hook(serviceName(opts.ServiceName))
	return gw.ListenAndServe(ctx)
}

func serviceName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "daigate"
	}
	return strings.TrimSpace(name)
}

func openGateway(opts ServeOptions) (*Gateway, error) {
	if opts.Registry == nil {
		return nil, fmt.Errorf("registry builder required")
	}
	cfgPath := resolveConfigPath(opts.ConfigPath)
	cfgFile, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if _, err := config.BrokerKey(cfgFile); err != nil {
		return nil, err
	}
	resolvePaths(cfgFile, cfgPath)

	loadCatalog := opts.CatalogLoad
	if loadCatalog == nil {
		loadCatalog = catalog.Load
	}
	cat, err := loadCatalog(cfgFile.Serve.Catalog)
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	st, ks, err := OpenStore(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}

	reg, err := opts.Registry(cfgFile)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("adapters: %w", err)
	}
	db, err := store.BrokerDB(st)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("broker db: %w", err)
	}
	auth, err := adminauth.Load(db, cfgFile)
	if err != nil {
		st.Close()
		return nil, fmt.Errorf("admin auth: %w", err)
	}

	return New(Config{
		ConfigFile:   cfgPath,
		Config:       cfgFile,
		Catalog:      cat,
		Store:        st,
		KeyStore:     ks,
		Adapters:     reg,
		AdminAuth:    auth,
		AdminEnabled: cfgFile.AdminPlaneEnabled(),
		DataListen:   cfgFile.Serve.DataListen,
		AdminListen:  cfgFile.Admin.Listen,
		DataMount:       opts.DataMount,
		WrapDataHandler: opts.WrapDataHandler,
	})
}

func resolveConfigPath(path string) string {
	if strings.TrimSpace(path) != "" {
		return strings.TrimSpace(path)
	}
	if _, err := os.Stat("daigate.yaml"); err == nil {
		return "daigate.yaml"
	}
	return "daigate.yaml"
}

func resolvePaths(cfg *config.File, configPath string) {
	base := filepath.Dir(configPath)
	if !filepath.IsAbs(cfg.Serve.Catalog) {
		cfg.Serve.Catalog = filepath.Join(base, cfg.Serve.Catalog)
	}
	if !filepath.IsAbs(cfg.Credential.Broker) {
		cfg.Credential.Broker = filepath.Join(base, cfg.Credential.Broker)
	}
}