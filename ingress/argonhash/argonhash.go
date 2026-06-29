// Package argonhash seals secrets with argon2id and a per-secret random salt.
package argonhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"golang.org/x/crypto/argon2"
)

const version = "v1"

// Seal returns an encoded sealed secret: v1$<salt_b64>$<hash_b64>.
func Seal(secret string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(secret), salt, 2, 64*1024, 4, 32)
	return version + "$" +
		base64.StdEncoding.EncodeToString(salt) + "$" +
		base64.StdEncoding.EncodeToString(sum), nil
}

// Verify checks a plaintext secret against a sealed value.
func Verify(secret, sealed string) bool {
	parts := strings.Split(sealed, "$")
	if len(parts) != 3 || parts[0] != version {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(secret), salt, 2, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}