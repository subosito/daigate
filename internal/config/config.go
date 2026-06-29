package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// OAuthProfile is generic OAuth2 endpoints for a credential profile.
type OAuthProfile struct {
	AuthorizeURL       string   `yaml:"authorize_url"`
	TokenURL           string   `yaml:"token_url"`
	DeviceAuthorizeURL string   `yaml:"device_authorize_url,omitempty"`
	DeviceTokenURL     string   `yaml:"device_token_url,omitempty"`
	ClientID           string   `yaml:"client_id,omitempty"`
	Scopes             []string `yaml:"scopes,omitempty"`
	PKCE               bool     `yaml:"pkce,omitempty"`
}

const (
	BrokerKeyEnv       = "DAIGATE_BROKER_KEY"
	legacyBrokerKeyEnv = "DAIGATE_VAULT_KEY" // deprecated alias — read when BrokerKeyEnv unset
)

// MintPolicyConfig caps gateway key minting on admin/provision routes.
type MintPolicyConfig struct {
	MaxTTL string   `yaml:"max_ttl,omitempty"`
	Scopes []string `yaml:"scopes,omitempty"`
}

// IssuerEntry is one self-service gateway key issuer plugin.
// Driver-specific settings live in Config; linked issuer plugins decode per driver.
type IssuerEntry struct {
	Driver string    `yaml:"driver"`
	Config yaml.Node `yaml:"config,omitempty"`
}

// Profile is a credential profile definition.
type Profile struct {
	Kind  string       `yaml:"kind"`
	OAuth OAuthProfile `yaml:"oauth,omitempty"`
}

// File is daigate.yaml root.
type File struct {
	Serve struct {
		DataListen string `yaml:"data_listen"`
		Catalog    string `yaml:"catalog"`
	} `yaml:"serve"`
	Admin struct {
		Listen string `yaml:"listen"`
		Enable *bool  `yaml:"enable,omitempty"`
		Tokens struct {
			AdminEnv     string `yaml:"admin_env"`
			ProvisionEnv string `yaml:"provision_env"`
		} `yaml:"tokens"`
		Keys      MintPolicyConfig `yaml:"keys,omitempty"`
		Provision MintPolicyConfig `yaml:"provision,omitempty"`
	} `yaml:"admin"`
	Credential struct {
		Backend    string    `yaml:"backend,omitempty"`
		Broker     string    `yaml:"broker"`
		Encryption struct {
			KeyEnv  string `yaml:"key_env"`
			KeyFile string `yaml:"key_file"`
		} `yaml:"encryption"`
		BackendConfig yaml.Node `yaml:"backend_config,omitempty"`
	} `yaml:"credential"`
	Ingress struct {
		ClientAuth string         `yaml:"client_auth"`
		Issuers    []IssuerEntry `yaml:"issuers,omitempty"`
	} `yaml:"ingress"`
	Adapters struct {
		Enable []string `yaml:"enable"`
	} `yaml:"adapters"`
	CredentialProfiles map[string]Profile `yaml:"credential_profiles"`
}

// Load reads and normalizes daigate.yaml.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	if f.Serve.DataListen == "" {
		f.Serve.DataListen = ":9420"
	}
	if f.Serve.Catalog == "" {
		f.Serve.Catalog = "providers.yaml"
	}
	if f.Admin.Listen == "" {
		f.Admin.Listen = "127.0.0.1:9421"
	}
	if f.Admin.Tokens.AdminEnv == "" {
		f.Admin.Tokens.AdminEnv = "DAIGATE_ADMIN_TOKEN"
	}
	if f.Admin.Tokens.ProvisionEnv == "" {
		f.Admin.Tokens.ProvisionEnv = "DAIGATE_PROVISION_TOKEN"
	}
	if f.Credential.Backend == "" {
		f.Credential.Backend = "sqlite"
	}
	if f.Credential.Broker == "" {
		f.Credential.Broker = "broker.db"
	}
	if f.Credential.Encryption.KeyEnv == "" {
		f.Credential.Encryption.KeyEnv = BrokerKeyEnv
	}
	if f.Ingress.ClientAuth == "" {
		f.Ingress.ClientAuth = "keyring"
	}
	if len(f.Adapters.Enable) == 0 {
		f.Adapters.Enable = []string{"passthrough"}
	}
	return &f, nil
}

// BrokerKey reads the broker.db AES encryption key from env or file.
func BrokerKey(f *File) (string, error) {
	if f.Credential.Encryption.KeyFile != "" {
		raw, err := os.ReadFile(f.Credential.Encryption.KeyFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(raw)), nil
	}
	envName := f.Credential.Encryption.KeyEnv
	v := strings.TrimSpace(os.Getenv(envName))
	if v == "" && envName == BrokerKeyEnv {
		v = strings.TrimSpace(os.Getenv(legacyBrokerKeyEnv))
	}
	if v == "" {
		return "", fmt.Errorf("broker encryption key required: set %s or credential.encryption.key_file", envName)
	}
	return v, nil
}

// AdminPlaneEnabled reports whether the admin listener and auth are active.
func (f *File) AdminPlaneEnabled() bool {
	if f.Admin.Enable != nil {
		return *f.Admin.Enable
	}
	return true
}

// AdminToken returns admin bearer token from env.
func AdminToken(f *File) (string, error) {
	v := strings.TrimSpace(os.Getenv(f.Admin.Tokens.AdminEnv))
	if v == "" {
		return "", fmt.Errorf("admin token required: set %s", f.Admin.Tokens.AdminEnv)
	}
	return v, nil
}

// ProvisionToken returns provision bearer token from env.
func ProvisionToken(f *File) (string, error) {
	v := strings.TrimSpace(os.Getenv(f.Admin.Tokens.ProvisionEnv))
	if v == "" {
		return "", fmt.Errorf("provision token required: set %s", f.Admin.Tokens.ProvisionEnv)
	}
	return v, nil
}