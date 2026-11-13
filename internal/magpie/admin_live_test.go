package magpie

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAdminClientGatewayKeyLifecycleLive(t *testing.T) {
	if os.Getenv("MAGPIE_LIVE_TEST") != "1" {
		t.Skip("set MAGPIE_LIVE_TEST=1 to run against the local Magpie gateway")
	}

	store, err := DefaultCredentialStore()
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	baseURL := strings.TrimSpace(os.Getenv("MAGPIE_LIVE_URL"))
	if baseURL == "" {
		baseURL = "http://127.0.0.1:3425"
	}
	client := NewAdminClient(baseURL, credentials.Secret)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	name := "CodexLink Lifecycle Test " + time.Now().Format("20060102-150405")
	added, err := client.AddGatewayKey(ctx, name, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(added.Secret, "sk-magpie-key-") {
		t.Fatal("Magpie did not return a valid gateway secret")
	}

	keyID := ""
	for _, key := range added.Keys {
		if key.Name == name {
			keyID = key.ID
			break
		}
	}
	if keyID == "" {
		keys, listErr := client.GatewayKeys(ctx)
		if listErr != nil {
			t.Fatal(listErr)
		}
		for _, key := range keys {
			if key.Name == name {
				keyID = key.ID
				break
			}
		}
	}
	if keyID == "" {
		t.Fatal("created gateway key was not found")
	}
	t.Cleanup(func() {
		if keyID == "" {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, removeErr := client.RemoveGatewayKey(cleanupCtx, keyID); removeErr != nil {
			t.Logf("cleanup remove failed: %v", removeErr)
		}
	})

	rotated, err := client.RotateGatewayKey(ctx, keyID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rotated.Secret, "sk-magpie-key-") || rotated.Secret == added.Secret {
		t.Fatal("gateway key rotation did not return a new secret")
	}

	if _, err := client.SetGatewayKeyEnabled(ctx, keyID, false); err != nil {
		t.Fatal(err)
	}
	keys, err := client.GatewayKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	off, found := gatewayKeyState(keys, keyID)
	if !found || !off {
		t.Fatal("gateway key was not disabled")
	}

	if _, err := client.SetGatewayKeyEnabled(ctx, keyID, true); err != nil {
		t.Fatal(err)
	}
	keys, err = client.GatewayKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	off, found = gatewayKeyState(keys, keyID)
	if !found || off {
		t.Fatal("gateway key was not re-enabled")
	}

	if _, err := client.RemoveGatewayKey(ctx, keyID); err != nil {
		t.Fatal(err)
	}
	keys, err = client.GatewayKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if key.ID == keyID {
			t.Fatal("gateway key still exists after removal")
		}
	}
	keyID = ""
}

func gatewayKeyState(keys []AdminGatewayKey, id string) (bool, bool) {
	for _, key := range keys {
		if key.ID == id {
			return key.Off, true
		}
	}
	return false, false
}
