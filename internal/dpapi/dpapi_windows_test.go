//go:build windows

package dpapi

import (
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	plain := []byte(`{"token":"sk-magpie-key-this-is-not-plaintext"}`)
	cipher, err := Protect(plain, []byte("CodexLink test entropy"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cipher, plain) {
		t.Fatal("DPAPI output contains plaintext")
	}
	decoded, err := Unprotect(cipher, []byte("CodexLink test entropy"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, plain) {
		t.Fatal("DPAPI round trip changed the payload")
	}
	if _, err := Unprotect(cipher, []byte("wrong entropy")); err == nil {
		t.Fatal("wrong entropy was accepted")
	}
}
