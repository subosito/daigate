package argonhash_test

import (
	"strings"
	"testing"

	"github.com/subosito/daigate/ingress/argonhash"
)

func TestSealVerifyRoundTrip(t *testing.T) {
	sealed, err := argonhash.Seal("sk-dg-test-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, "v1$") {
		t.Fatalf("sealed=%q", sealed)
	}
	if !argonhash.Verify("sk-dg-test-secret-value", sealed) {
		t.Fatal("verify failed")
	}
	if argonhash.Verify("wrong", sealed) {
		t.Fatal("wrong secret accepted")
	}
}

func TestDistinctSalts(t *testing.T) {
	a, _ := argonhash.Seal("same")
	b, _ := argonhash.Seal("same")
	if a == b {
		t.Fatal("sealed values must differ per salt")
	}
}