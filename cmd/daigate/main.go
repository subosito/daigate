package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 0
	}
	switch args[0] {
	case "serve":
		return serveCmd(args[1:])
	case "credential":
		return credentialCmd(args[1:])
	case "keys":
		return keysCmd(args[1:])
	case "adapters":
		return adaptersCmd(args[1:])
	case "admin":
		return adminCmd(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "daigate: unknown command %q\n", args[0])
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `daigate — the composable AI gateway

Usage:
  daigate serve [--config daigate.yaml]
  daigate credential list|show|import|login|refresh|disable
  daigate keys create [--static] [--name NAME] [--scopes wire:…,model:…]
  daigate adapters list|doctor
  daigate admin token create --role admin|provision

`)
}