package config_test

import (
	"testing"

	"github.com/subosito/daigate/internal/config"
)

func TestBrokerKeyRequired(t *testing.T) {
	f := &config.File{}
	f.Credential.Encryption.KeyEnv = "DAIGATE_BROKER_KEY_TEST_MISSING"
	t.Setenv("DAIGATE_BROKER_KEY_TEST_MISSING", "")
	if _, err := config.BrokerKey(f); err == nil {
		t.Fatal("expected broker key error")
	}
}

func TestBrokerKeyFromEnv(t *testing.T) {
	f := &config.File{}
	f.Credential.Encryption.KeyEnv = "DAIGATE_BROKER_KEY_TEST"
	t.Setenv("DAIGATE_BROKER_KEY_TEST", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	key, err := config.BrokerKey(f)
	if err != nil {
		t.Fatalf("broker key: %v", err)
	}
	if key == "" {
		t.Fatal("empty key")
	}
}

func TestBrokerKeyLegacyVaultEnvAlias(t *testing.T) {
	f := &config.File{}
	f.Credential.Encryption.KeyEnv = config.BrokerKeyEnv
	t.Setenv(config.BrokerKeyEnv, "")
	t.Setenv("DAIGATE_VAULT_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	key, err := config.BrokerKey(f)
	if err != nil {
		t.Fatalf("broker key: %v", err)
	}
	if key == "" {
		t.Fatal("empty key")
	}
}