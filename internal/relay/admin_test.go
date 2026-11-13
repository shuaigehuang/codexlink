package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func loginTestAdmin(t *testing.T, httpServerURL string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response, err := client.PostForm(httpServerURL+PathAdmin+"/login", url.Values{
		"password": {testAdminToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin login status=%d body=%s", response.StatusCode, body)
	}
	match := regexp.MustCompile(`name="csrf-token" content="([^"]+)"`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("csrf token not found in admin page: %s", body)
	}
	return client, string(match[1])
}

func adminAPIRequest(t *testing.T, client *http.Client, method, url, csrf string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestAdminTokenManagementLifecycle(t *testing.T) {
	_, httpServer := newTestRelay(t, 10*time.Minute, 90*24*time.Hour)

	loginPage, err := http.Get(httpServer.URL + PathAdmin)
	if err != nil {
		t.Fatal(err)
	}
	loginBody, _ := io.ReadAll(loginPage.Body)
	loginPage.Body.Close()
	if loginPage.StatusCode != http.StatusOK || !bytes.Contains(loginBody, []byte("管理员登录")) {
		t.Fatalf("unexpected login page status=%d body=%s", loginPage.StatusCode, loginBody)
	}

	client, csrf := loginTestAdmin(t, httpServer.URL)
	stateResponse := adminAPIRequest(t, client, http.MethodGet, httpServer.URL+PathAdmin+"/api/state", "", nil)
	if stateResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(stateResponse.Body)
		stateResponse.Body.Close()
		t.Fatalf("state status=%d body=%s", stateResponse.StatusCode, body)
	}
	var state adminStateResponse
	if err := json.NewDecoder(stateResponse.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	stateResponse.Body.Close()
	if state.TokenCount != 1 || len(state.Addresses) != 2 || !state.Addresses[0].Secure || !state.Addresses[0].Recommended {
		t.Fatalf("unexpected initial state: %+v", state)
	}

	withoutCSRF := adminAPIRequest(t, client, http.MethodPost, httpServer.URL+PathAdmin+"/api/tokens", "",
		map[string]string{"name": "Rejected"})
	if withoutCSRF.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(withoutCSRF.Body)
		withoutCSRF.Body.Close()
		t.Fatalf("missing CSRF status=%d body=%s", withoutCSRF.StatusCode, body)
	}
	withoutCSRF.Body.Close()

	createResponse := adminAPIRequest(t, client, http.MethodPost, httpServer.URL+PathAdmin+"/api/tokens",
		csrf, map[string]string{"name": "Second PC"})
	if createResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResponse.Body)
		createResponse.Body.Close()
		t.Fatalf("create token status=%d body=%s", createResponse.StatusCode, body)
	}
	var created struct {
		Token  string         `json:"token"`
		Record adminTokenView `json:"record"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	createResponse.Body.Close()
	if len(created.Token) < minDesktopTokenLen || created.Record.ID == "" {
		t.Fatalf("incomplete created token: %+v", created)
	}

	desktop := dialWebSocket(t, wsURL(httpServer, PathDesktopConnect+"?device=second-pc&name=Second"),
		created.Token)
	hello := readWebSocket(t, desktop)
	if hello.Type != MessageTypeHello {
		t.Fatalf("unexpected desktop hello: %+v", hello)
	}

	stateResponse = adminAPIRequest(t, client, http.MethodGet, httpServer.URL+PathAdmin+"/api/state", "", nil)
	if err := json.NewDecoder(stateResponse.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	stateResponse.Body.Close()
	if state.ActiveDesktops != 1 || state.TokenCount != 2 {
		t.Fatalf("unexpected state after desktop connect: %+v", state)
	}

	deleteResponse := adminAPIRequest(t, client, http.MethodDelete,
		httpServer.URL+PathAdmin+"/api/tokens/"+created.Record.ID, csrf, nil)
	if deleteResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(deleteResponse.Body)
		deleteResponse.Body.Close()
		t.Fatalf("delete token status=%d body=%s", deleteResponse.StatusCode, body)
	}
	deleteResponse.Body.Close()

	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if _, _, err := desktop.Read(readCtx); err != nil {
			break
		}
	}

	stateResponse = adminAPIRequest(t, client, http.MethodGet, httpServer.URL+PathAdmin+"/api/state", "", nil)
	if err := json.NewDecoder(stateResponse.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	stateResponse.Body.Close()
	if state.TokenCount != 1 {
		t.Fatalf("token count after delete = %d, want 1", state.TokenCount)
	}

	lastDelete := adminAPIRequest(t, client, http.MethodDelete,
		httpServer.URL+PathAdmin+"/api/tokens/"+state.Tokens[0].ID, csrf, nil)
	if lastDelete.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(lastDelete.Body)
		lastDelete.Body.Close()
		t.Fatalf("last token delete status=%d body=%s", lastDelete.StatusCode, body)
	}
	lastDelete.Body.Close()
}

func TestAdminLoginRejectsWrongPassword(t *testing.T) {
	_, httpServer := newTestRelay(t, 10*time.Minute, 90*24*time.Hour)
	response, err := http.PostForm(httpServer.URL+PathAdmin+"/login", url.Values{
		"password": {"wrong-password"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password status=%d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestDesktopTokenStorePersistsAndDoesNotReimportInstallerToken(t *testing.T) {
	const initialToken = "initial-desktop-token-0123456789abcdef"
	dir := t.TempDir()
	store, err := newDesktopTokenStore(dir, initialToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Authenticate(initialToken); !ok {
		t.Fatal("initial token was not imported")
	}
	records := store.List()
	if len(records) != 1 {
		t.Fatalf("initial token count = %d, want 1", len(records))
	}
	added, secret, err := store.Add("第二台电脑", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Delete(records[0].ID); err != nil {
		t.Fatal(err)
	}

	reloaded, err := newDesktopTokenStore(dir, initialToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Authenticate(initialToken); ok {
		t.Fatal("deleted installer token was reimported after restart")
	}
	if _, ok := reloaded.Authenticate(secret); !ok {
		t.Fatal("persisted admin-created token could not authenticate")
	}
	if _, err := reloaded.Delete(added.ID); !errors.Is(err, errLastToken) {
		t.Fatalf("last token delete error = %v, want %v", err, errLastToken)
	}
}

func TestAdminAddressNormalization(t *testing.T) {
	got := normalizePublicURLs([]string{
		"http://Relay.Example.com/",
		"https://relay.example.com/path",
		"ftp://relay.example.com",
		"https://relay.example.com",
	})
	want := []string{"https://relay.example.com", "http://Relay.Example.com"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("normalizePublicURLs() = %v, want %v", got, want)
	}
}

func TestWebSocketCloseStatusAfterTokenDelete(t *testing.T) {
	_, httpServer := newTestRelay(t, 10*time.Minute, 90*24*time.Hour)
	client, csrf := loginTestAdmin(t, httpServer.URL)
	createResponse := adminAPIRequest(t, client, http.MethodPost, httpServer.URL+PathAdmin+"/api/tokens",
		csrf, map[string]string{"name": "Temporary"})
	var created struct {
		Token  string         `json:"token"`
		Record adminTokenView `json:"record"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	createResponse.Body.Close()

	conn := dialWebSocket(t, wsURL(httpServer, PathDesktopConnect+"?device=close-test&name=CloseTest"), created.Token)
	_ = readWebSocket(t, conn)
	deleteResponse := adminAPIRequest(t, client, http.MethodDelete,
		httpServer.URL+PathAdmin+"/api/tokens/"+created.Record.ID, csrf, nil)
	deleteResponse.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("close status = %v, want %v (err=%v)", websocket.CloseStatus(err), websocket.StatusPolicyViolation, err)
	}
}
