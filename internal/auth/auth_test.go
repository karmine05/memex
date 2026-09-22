package auth

import "testing"

func TestKeyIsHashedAndPrefixed(t *testing.T) {
	key, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) < 8 || key[:4] != PrefixKey {
		t.Fatal(key)
	}
	sum := Hash(key)
	if sum == key || !EqualHash(sum, Hash(key)) || EqualHash(sum, Hash(key+"x")) {
		t.Fatal("hash")
	}
	tok, err := NewToken()
	if err != nil || tok[:4] != PrefixToken {
		t.Fatal(tok, err)
	}
}
