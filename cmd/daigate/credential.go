package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/subosito/daigate/credential/oauth/generic"
	"github.com/subosito/daigate/credential/oauth/vendor"
	"github.com/subosito/daigate/credential/store"
	"github.com/subosito/daigate/gateway"
	"github.com/subosito/daigate/internal/config"
)

func credentialCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "daigate credential: subcommand required")
		return 2
	}
	switch args[0] {
	case "list":
		return credentialListCmd(args[1:])
	case "show":
		return credentialShowCmd(args[1:])
	case "import":
		return credentialImportCmd(args[1:])
	case "login":
		return credentialLoginCmd(args[1:])
	case "disable":
		return credentialDisableCmd(args[1:])
	case "refresh":
		return credentialRefreshCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "daigate credential: unknown subcommand %q\n", args[0])
		return 2
	}
}

func loadVault(configPath string) (*config.File, store.Store, func(), error) {
	cfgFile, err := config.Load(configPath)
	if err != nil {
		return nil, nil, nil, err
	}
	base := filepath.Dir(configPath)
	brokerPath := cfgFile.Credential.Broker
	if !filepath.IsAbs(brokerPath) {
		brokerPath = filepath.Join(base, brokerPath)
	}
	cfgFile.Credential.Broker = brokerPath
	st, _, err := gateway.OpenStore(cfgFile)
	if err != nil {
		return nil, nil, nil, err
	}
	return cfgFile, st, func() { _ = st.Close() }, nil
}

func credentialListCmd(args []string) int {
	fs := flag.NewFlagSet("credential list", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	_, st, closeFn, err := loadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential list: %v\n", err)
		return 1
	}
	defer closeFn()
	list, err := st.ListSummaries(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential list: %v\n", err)
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return boolToExit(enc.Encode(list))
}

func credentialShowCmd(args []string) int {
	fs := flag.NewFlagSet("credential show", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "daigate credential show: credential id required")
		return 2
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential show: invalid id: %v\n", err)
		return 2
	}
	_, st, closeFn, err := loadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential show: %v\n", err)
		return 1
	}
	defer closeFn()
	cs, err := st.GetSummary(context.Background(), id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential show: %v\n", err)
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return boolToExit(enc.Encode(cs))
}

func credentialImportCmd(args []string) int {
	fs := flag.NewFlagSet("credential import", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	apiKey := fs.String("api-key", "", "api key value")
	fs.SetOutput(os.Stderr)
	rest := flagsFirst(args, fs)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "daigate credential import: profile name required")
		return 2
	}
	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "daigate credential import: --api-key required")
		return 2
	}
	_, st, closeFn, err := loadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential import: %v\n", err)
		return 1
	}
	defer closeFn()
	profile := fs.Arg(0)
	id, err := st.PutAPIKey(context.Background(), profile, *apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential import: %v\n", err)
		return 1
	}
	fmt.Printf("imported id=%d profile=%s\n", id, profile)
	return 0
}

func credentialLoginCmd(args []string) int {
	fs := flag.NewFlagSet("credential login", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	flowFlag := fs.String("flow", "auto", "login flow: auto, browser, device, manual")
	callbackFlag := fs.String("callback-listen", "127.0.0.1:0", "loopback addr for browser OAuth callback (ephemeral port; does not use admin.listen)")
	fs.SetOutput(os.Stderr)
	rest := flagsFirst(args, fs)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "daigate credential login: profile name required")
		return 2
	}
	profile := fs.Arg(0)
	cfgFile, st, closeFn, err := loadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential login: %v\n", err)
		return 1
	}
	defer closeFn()
	prof, ok := cfgFile.CredentialProfiles[profile]
	if !ok || prof.Kind != "oauth" {
		fmt.Fprintf(os.Stderr, "daigate credential login: profile %q not configured as oauth in credential_profiles\n", profile)
		return 1
	}
	flow := generic.Flow(strings.ToLower(*flowFlag))
	callbackAddr := *callbackFlag
	ctrl := generic.Controller{
		OnAuth: func(info generic.AuthInfo) {
			fmt.Fprintln(os.Stderr)
			if info.UserCode != "" {
				fmt.Fprintf(os.Stderr, "User code: %s\n", info.UserCode)
			}
			if info.URL != "" {
				fmt.Fprintf(os.Stderr, "Open: %s\n", info.URL)
			}
			if info.Instructions != "" {
				fmt.Fprintln(os.Stderr, info.Instructions)
			}
		},
		OnProgress: func(msg string) { fmt.Fprintln(os.Stderr, msg) },
	}
	if flow == generic.FlowManual {
		ctrl.OnManualInput = func(ctx context.Context) (string, error) {
			fmt.Fprint(os.Stderr, "Paste redirect URL or authorization code: ")
			line, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(line), nil
		}
	}
	mat, err := vendor.Login(context.Background(), profile, prof, flow, callbackAddr, ctrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential login: %v\n", err)
		return 1
	}
	id, err := st.PutOAuth(context.Background(), profile, mat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential login: store: %v\n", err)
		return 1
	}
	fmt.Printf("logged in id=%d profile=%s flow=%s\n", id, profile, flow)
	return 0
}

func credentialRefreshCmd(args []string) int {
	fs := flag.NewFlagSet("credential refresh", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	fs.SetOutput(os.Stderr)
	rest := flagsFirst(args, fs)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "daigate credential refresh: profile name required")
		return 2
	}
	profile := fs.Arg(0)
	cfgFile, st, closeFn, err := loadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential refresh: %v\n", err)
		return 1
	}
	defer closeFn()
	prof, ok := cfgFile.CredentialProfiles[profile]
	if !ok || prof.Kind != "oauth" {
		fmt.Fprintf(os.Stderr, "daigate credential refresh: profile %q not configured as oauth\n", profile)
		return 1
	}
	cur, err := st.Get(context.Background(), profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential refresh: %v\n", err)
		return 1
	}
	mat, err := vendor.Refresh(context.Background(), profile, prof, cur)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential refresh: %v\n", err)
		return 1
	}
	if err := st.UpdateOAuth(context.Background(), profile, mat); err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential refresh: store: %v\n", err)
		return 1
	}
	fmt.Printf("refreshed profile=%s\n", profile)
	return 0
}

func credentialDisableCmd(args []string) int {
	fs := flag.NewFlagSet("credential disable", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	cause := fs.String("cause", "disabled by operator", "disable reason")
	fs.SetOutput(os.Stderr)
	rest := flagsFirst(args, fs)
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "daigate credential disable: credential id required")
		return 2
	}
	id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential disable: invalid id: %v\n", err)
		return 2
	}
	_, st, closeFn, err := loadVault(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential disable: %v\n", err)
		return 1
	}
	defer closeFn()
	if err := st.Disable(context.Background(), id, *cause); err != nil {
		fmt.Fprintf(os.Stderr, "daigate credential disable: %v\n", err)
		return 1
	}
	fmt.Printf("disabled id=%d\n", id)
	return 0
}

func boolToExit(err error) int {
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		return 1
	}
	return 0
}

func flagsFirst(args []string, fs *flag.FlagSet) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if takesValue(arg) && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
		} else {
			pos = append(pos, arg)
		}
	}
	_ = fs
	return append(flags, pos...)
}

func takesValue(arg string) bool {
	if arg == "-" || arg == "--" {
		return false
	}
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	if len(name) > 1 {
		return true
	}
	switch name {
	case "h", "v":
		return false
	default:
		return true
	}
}