package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	PrefixKey   = "mxk_"
	PrefixAdmin = "mxa_"
	PrefixToken = "mxt_"
)

func newSecret(prefix string) (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("secret: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func NewAPIKey() (string, error)   { return newSecret(PrefixKey) }
func NewAdminKey() (string, error) { return newSecret(PrefixAdmin) }
func NewToken() (string, error)    { return newSecret(PrefixToken) }

func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func EqualHash(a, b string) bool {
	ab, errA := hex.DecodeString(a)
	bb, errB := hex.DecodeString(b)
	if errA != nil || errB != nil || len(ab) != len(bb) {
		return false
	}
	return subtle.ConstantTimeCompare(ab, bb) == 1
}
