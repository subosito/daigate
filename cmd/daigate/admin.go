package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/gateway"
	"github.com/subosito/daigate/ingress/adminauth"
	"github.com/subosito/daigate/internal/config"
)

func adminCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "daigate admin: subcommand required")
		return 2
	}
	switch args[0] {
	case "token":
		return adminTokenCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "daigate admin: unknown subcommand %q\n", args[0])
		return 2
	}
}

func adminTokenCmd(args []string) int {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "daigate admin token: subcommand create required")
		return 2
	}
	fs := flag.NewFlagSet("admin token create", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	role := fs.String("role", "provision", "admin or provision")
	name := fs.String("name", "cli", "token name")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	cfgFile, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate admin token create: %v\n", err)
		return 1
	}
	base := filepath.Dir(*configPath)
	brokerPath := cfgFile.Credential.Broker
	if !filepath.IsAbs(brokerPath) {
		brokerPath = filepath.Join(base, brokerPath)
	}
	cfgFile.Credential.Broker = brokerPath

	st, _, err := gateway.OpenStore(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate admin token create: %v\n", err)
		return 1
	}
	defer st.Close()

	tok, err := randomToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate admin token create: %v\n", err)
		return 1
	}
	r := adminauth.Role(*role)
	if r != adminauth.RoleAdmin && r != adminauth.RoleProvision {
		fmt.Fprintf(os.Stderr, "daigate admin token create: role must be admin or provision\n")
		return 2
	}
	db, err := store.BrokerDB(st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate admin token create: %v\n", err)
		return 1
	}
	ts := adminauth.NewSQLTokenStore(db)
	if err := ts.Insert(context.Background(), *name, r, tok); err != nil {
		fmt.Fprintf(os.Stderr, "daigate admin token create: %v\n", err)
		return 1
	}
	fmt.Printf("name=%s role=%s token=%s\n", *name, r, tok)
	fmt.Fprintf(os.Stderr, "Token stored in broker.db; restart serve to activate (env %s still required)\n", envForRole(cfgFile, r))
	return 0
}

func envForRole(f *config.File, r adminauth.Role) string {
	if r == adminauth.RoleAdmin {
		return f.Admin.Tokens.AdminEnv
	}
	return f.Admin.Tokens.ProvisionEnv
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}