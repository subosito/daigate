package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestHelpListsSubcommands(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	code := run(nil)
	_ = w.Close()
	os.Stderr = old
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()
	for _, sub := range []string{"serve", "credential", "keys", "adapters", "admin"} {
		if !strings.Contains(out, sub) {
			t.Fatalf("help missing %q: %s", sub, out)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	if code := run([]string{"nope"}); code != 2 {
		t.Fatalf("exit=%d", code)
	}
}