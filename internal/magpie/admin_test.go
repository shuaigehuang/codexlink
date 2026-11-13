package magpie

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminClientSnapshot(t *testing.T) {
	const secret = "sk-magpie-key-test-admin"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Errorf("Authorization = %q", got)
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			t.Errorf("browser security headers must not be sent")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/magpie/admin/providers":
			io.WriteString(w, `{"providers":[{"id":"deepseek","name":"DeepSeek","preset":"deepseek","host":"api.deepseek.com","ready":true,"key":{"set":true,"masked":"sk-1…2603"},"keyList":[{"id":"abc123","masked":"sk-1…2603","active":true,"on":true}],"models":[{"id":"a"},{"id":"b"}]}]}`)
		case "/v1/magpie/admin/gateway-keys":
			io.WriteString(w, `{"keys":[{"id":"key-1","name":"CodexLink Desktop Admin","masked":"sk-magpie-key-…123456","admin":"local"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	snapshot, err := NewAdminClient(server.URL, secret).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Providers) != 1 || snapshot.Providers[0].ID != "deepseek" {
		t.Fatalf("providers = %+v", snapshot.Providers)
	}
	if snapshot.Providers[0].Models != 2 || !snapshot.Providers[0].KeySet {
		t.Fatalf("provider details = %+v", snapshot.Providers[0])
	}
	if len(snapshot.GatewayKeys) != 1 || snapshot.GatewayKeys[0].Admin != "local" {
		t.Fatalf("gateway keys = %+v", snapshot.GatewayKeys)
	}
}

func TestAdminClientGatewayKeyLifecycle(t *testing.T) {
	const secret = "sk-magpie-key-test-admin"
	var actions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		actions = append(actions, r.URL.Path+"|"+string(body))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/magpie/admin/gateway-keys/add-key":
			if request["name"] != "Phone" || request["readOnly"] != false {
				t.Errorf("add body = %s", body)
			}
			io.WriteString(w, `{"keys":[{"id":"child-1","name":"Phone"}],"secret":"sk-magpie-key-child-secret"}`)
		case "/v1/magpie/admin/gateway-keys/rotate-key":
			io.WriteString(w, `{"keys":[{"id":"child-1","name":"Phone"}],"secret":"sk-magpie-key-child-rotated"}`)
		default:
			io.WriteString(w, `{"keys":[]}`)
		}
	}))
	defer server.Close()

	client := NewAdminClient(server.URL+"/", secret)
	added, err := client.AddGatewayKey(context.Background(), "Phone", false)
	if err != nil || added.Secret != "sk-magpie-key-child-secret" {
		t.Fatalf("added = %+v, err = %v", added, err)
	}
	rotated, err := client.RotateGatewayKey(context.Background(), "child-1")
	if err != nil || rotated.Secret != "sk-magpie-key-child-rotated" {
		t.Fatalf("rotated = %+v, err = %v", rotated, err)
	}
	if _, err := client.SetGatewayKeyEnabled(context.Background(), "child-1", false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RemoveGatewayKey(context.Background(), "child-1"); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 4 {
		t.Fatalf("actions = %v", actions)
	}
	for _, action := range actions {
		if strings.Contains(action, "admin-key") || strings.Contains(action, "copy-key") {
			t.Fatalf("admin client used a forbidden action: %s", action)
		}
	}
}

func TestReplaceProviderKeyErrorRedactsSecret(t *testing.T) {
	const providerKey = "relay-provider-key-very-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"rejected relay-provider-key-very-secret"}`)
	}))
	defer server.Close()

	err := NewAdminClient(server.URL, "sk-magpie-key-admin").ReplaceProviderKey(
		context.Background(),
		"relay",
		"key-ref",
		providerKey,
	)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), providerKey) {
		t.Fatalf("error leaked provider key: %v", err)
	}
}

func TestFindGatewayKeyID(t *testing.T) {
	output := `ID                        NAME                       STATE
38ae1bd366e74772dec237bb  CodexLink Desktop Admin
aaaaaaaaaaaaaaaaaaaaaaaa  Other`
	id, ok := findGatewayKeyID(output, AdminKeyName)
	if !ok || id != "38ae1bd366e74772dec237bb" {
		t.Fatalf("id = %q, ok = %v", id, ok)
	}
}
