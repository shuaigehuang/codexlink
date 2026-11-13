//go:build windows

package magpie

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialStoreRoundTripUsesDPAPI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	store := NewCredentialStore(path)
	want := AdminCredentials{
		KeyID:   "38ae1bd366e74772dec237bb",
		KeyName: AdminKeyName,
		Secret:  "sk-magpie-key-this-must-not-be-plaintext-0123456789abcdef",
	}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(want.Secret)) {
		t.Fatal("credentials file contains the plaintext secret")
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loaded = %+v, want %+v", got, want)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err != ErrCredentialsNotFound {
		t.Fatalf("after delete: %v", err)
	}
}

func TestCredentialStoreRejectsCorruptData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	if err := os.WriteFile(path, []byte("not encrypted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCredentialStore(path).Load(); err == nil {
		t.Fatal("corrupt credential file was accepted")
	}
}

func TestCredentialStoreFallsBackWhenRenameFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.dat")
	want := AdminCredentials{
		KeyID:   "38ae1bd366e74772dec237bb",
		KeyName: AdminKeyName,
		Secret:  "sk-magpie-key-fallback-must-stay-encrypted-0123456789",
	}
	original := renameCredentialFile
	renameCredentialFile = func(string, string) error {
		return errors.New("simulated rename failure")
	}
	t.Cleanup(func() {
		renameCredentialFile = original
	})

	store := NewCredentialStore(path)
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(want.Secret)) {
		t.Fatal("fallback credentials file contains the plaintext secret")
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loaded = %+v, want %+v", got, want)
	}
}
