package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/subosito/daigate/adaptersdk"
	"github.com/subosito/daigate/compose"
	"github.com/subosito/daigate/gateway"
)

func serveCmd(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	err := gateway.Serve(context.Background(), gateway.ServeOptions{
		ConfigPath: *configPath,
		Registry: func(cfg *gateway.ConfigFile) (*adaptersdk.Registry, error) {
			return compose.FromConfig(cfg.Adapters.Enable, compose.DefaultAdapters())
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate serve: %v\n", err)
		return 1
	}
	return 0
}