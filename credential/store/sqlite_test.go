package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subosito/daigate/credential/seal"
	"github.com/subosito/daigate/credential/store"
)

func TestSQLiteEncryptAtRest(t *testing.T) {
	key, _ := seal.ParseKey("CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=")
	path := filepath.Join(t.TempDir(), "broker.db")
	st, err := store.OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_, err = st.PutAPIKey(context.Background(), "mock", "sk-secret-key-value")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-secret") {
		t.Fatal("broker.db must not contain plaintext secret")
	}
	list, err := st.ListSummaries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Profile != "mock" {
		t.Fatalf("summary: %+v", list)
	}
	mat, err := st.Get(context.Background(), "mock")
	if err != nil || mat.APIKey != "sk-secret-key-value" {
		t.Fatalf("get: %v %+v", err, mat)
	}
}