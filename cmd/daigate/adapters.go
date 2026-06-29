package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/subosito/daigate/catalog"
	"github.com/subosito/daigate/compose"
	"github.com/subosito/daigate/internal/config"
)

func adaptersCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "daigate adapters: subcommand required")
		return 2
	}
	switch args[0] {
	case "list":
		return adaptersListCmd(args[1:])
	case "doctor":
		return adaptersDoctorCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "daigate adapters: unknown subcommand %q\n", args[0])
		return 2
	}
}

func adaptersListCmd(args []string) int {
	fs := flag.NewFlagSet("adapters list", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfgFile, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate adapters list: %v\n", err)
		return 1
	}
	reg, err := compose.FromConfig(cfgFile.Adapters.Enable, compose.DefaultAdapters())
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate adapters list: %v\n", err)
		return 1
	}
	type entry struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	var out []entry
	for p := range reg.ChatHandlers {
		out = append(out, entry{Name: p, Kind: "chat-protocol"})
	}
	for p := range reg.EmbedHandlers {
		out = append(out, entry{Name: p, Kind: "embed-protocol"})
	}
	for p := range reg.ImageHandlers {
		out = append(out, entry{Name: p, Kind: "image-protocol"})
	}
	for p := range reg.SpeechHandlers {
		out = append(out, entry{Name: p, Kind: "speech-protocol"})
	}
	for p := range reg.VideoHandlers {
		out = append(out, entry{Name: p, Kind: "video-protocol"})
	}
	for a := range reg.ImageAdapters {
		out = append(out, entry{Name: a, Kind: "image-adapter"})
	}
	for a := range reg.SpeechAdapters {
		out = append(out, entry{Name: a, Kind: "speech-adapter"})
	}
	for a := range reg.EmbedAdapters {
		out = append(out, entry{Name: a, Kind: "embed-adapter"})
	}
	for a := range reg.VideoAdapters {
		out = append(out, entry{Name: a, Kind: "video-adapter"})
	}
	for a := range reg.ChatAdapters {
		out = append(out, entry{Name: a, Kind: "chat-adapter"})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return boolToExit(enc.Encode(out))
}

func adaptersDoctorCmd(args []string) int {
	fs := flag.NewFlagSet("adapters doctor", flag.ExitOnError)
	configPath := fs.String("config", "daigate.yaml", "path to daigate.yaml")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfgFile, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate adapters doctor: %v\n", err)
		return 1
	}
	base := filepath.Dir(*configPath)
	catalogPath := cfgFile.Serve.Catalog
	if !filepath.IsAbs(catalogPath) {
		catalogPath = filepath.Join(base, catalogPath)
	}
	cat, err := catalog.Load(catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate adapters doctor: %v\n", err)
		return 1
	}
	reg, err := compose.FromConfig(cfgFile.Adapters.Enable, compose.DefaultAdapters())
	if err != nil {
		fmt.Fprintf(os.Stderr, "daigate adapters doctor: %v\n", err)
		return 1
	}
	protocols := make(map[string]bool)
	for p := range reg.ChatHandlers {
		protocols[p] = true
	}
	for p := range reg.EmbedHandlers {
		protocols[p] = true
	}
	for p := range reg.ImageHandlers {
		protocols[p] = true
	}
	for p := range reg.SpeechHandlers {
		protocols[p] = true
	}
	for p := range reg.VideoHandlers {
		protocols[p] = true
	}
	adapters := make(map[string]bool)
	for a := range reg.ImageAdapters {
		adapters[a] = true
	}
	for a := range reg.SpeechAdapters {
		adapters[a] = true
	}
	for a := range reg.EmbedAdapters {
		adapters[a] = true
	}
	for a := range reg.VideoAdapters {
		adapters[a] = true
	}
	for a := range reg.ChatAdapters {
		adapters[a] = true
	}
	missing := catalog.Doctor(cat, protocols, adapters)
	if len(missing) > 0 {
		sort.Strings(missing)
		for _, p := range missing {
			fmt.Fprintf(os.Stderr, "missing handler for %s\n", p)
		}
		return 1
	}
	fmt.Println("ok")
	return 0
}