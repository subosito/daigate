package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/subosito/daigate/credential/seal"
)

type apiPayload struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

type oauthPayload struct {
	Type    string `json:"type"`
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
	Expires int64  `json:"expires"`
	Email   string `json:"email,omitempty"`
	Account string `json:"account_id,omitempty"`
	Project string `json:"project_id,omitempty"`
}

// EncryptPayload JSON-marshals v and encrypts with key.
func EncryptPayload(key seal.Key, v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return key.Encrypt(raw)
}

// MaterialFromDecrypted builds Material from decrypted credential JSON.
func MaterialFromDecrypted(profile string, kind Kind, raw []byte) (Material, error) {
	mat := Material{Profile: profile, Kind: kind}
	switch kind {
	case KindAPIKey:
		var p apiPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return Material{}, err
		}
		mat.APIKey = p.Key
	case KindOAuth:
		var p oauthPayload
		if err := json.Unmarshal(raw, &p); err != nil {
			return Material{}, err
		}
		mat.AccessToken = p.Access
		mat.RefreshToken = p.Refresh
		if p.Expires > 0 {
			mat.ExpiresAt = time.UnixMilli(p.Expires)
		}
		mat.Email = p.Email
		mat.AccountID = p.Account
		mat.ProjectID = p.Project
	default:
		return Material{}, fmt.Errorf("unknown kind %q", kind)
	}
	return mat, nil
}

func apiKeyPayload(key string) apiPayload {
	return apiPayload{Type: "api_key", Key: key}
}

func oauthMaterialPayload(mat Material) oauthPayload {
	var exp int64
	if !mat.ExpiresAt.IsZero() {
		exp = mat.ExpiresAt.UnixMilli()
	}
	return oauthPayload{
		Type: "oauth", Access: mat.AccessToken, Refresh: mat.RefreshToken,
		Expires: exp, Email: mat.Email, Account: mat.AccountID, Project: mat.ProjectID,
	}
}