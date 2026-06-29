package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/subosito/daigate/gateway"
	"github.com/subosito/daigate/ingress/keyring"
	"github.com/subosito/daigate/internal/config"
)

func keysCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "daigate keys: subcommand required (create)")
		return 2
	}
	switch args[0] {
	case "create":
		return keysCreateCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "daigate keys: unknown subcommand %q\n", args[0])
		return 2
	}
}

func keysCreateCmd(args []string) int {
	fs := flag.NewFlagSet("keys create", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	name := fs.String("name", "default", "gateway key name")
	static := fs.Bool("static", false, "create static (non-expiring) key")
	ttlStr := fs.String("ttl", "720h", "TTL for issued keys")
	scopesStr := fs.String("scopes", "", "comma-separated scopes (model:id, wire:id, or *)")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfgFile, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate keys create: %v\n", err)
		return 1
	}
	base := filepath.Dir(*configPath)
	brokerPath := cfgFile.Credential.Broker
	if !filepath.IsAbs(brokerPath) {
		brokerPath = filepath.Join(base, brokerPath)
	}
	cfgFile.Credential.Broker = brokerPath

	st, ks, err := gateway.OpenStore(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate keys create: %v\n", err)
		return 1
	}
	defer st.Close()

	kind := keyring.KindIssued
	var ttl time.Duration
	if *static {
		kind = keyring.KindStatic
	} else {
		ttl, err = time.ParseDuration(*ttlStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "daigate keys create: ttl: %v\n", err)
			return 1
		}
	}

	var scopes []string
	if *scopesStr != "" {
		for _, s := range strings.Split(*scopesStr, ",") {
			if t := strings.TrimSpace(s); t != "" {
				scopes = append(scopes, t)
			}
		}
	}
	secret, id, err := ks.Create(context.Background(), *name, kind, ttl, scopes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate keys create: %v\n", err)
		return 1
	}
	fmt.Printf("id=%d name=%s kind=%s key=%s\n", id, *name, kind, secret)
	return 0
}