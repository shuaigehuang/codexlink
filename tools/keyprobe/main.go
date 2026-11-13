package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	relayproto "github.com/shuaigehuang/codexlink/internal/relay"

	"github.com/coder/websocket"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: keyprobe <relay-url> <pair-code> [phone-name]")
		os.Exit(2)
	}
	relayURL := strings.TrimRight(os.Args[1], "/")
	pairCode := strings.ReplaceAll(os.Args[2], " ", "")
	phoneName := "CodexLink Key Probe"
	if len(os.Args) > 3 {
		phoneName = os.Args[3]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	claimed, err := claimPair(ctx, relayURL, pairCode, phoneName)
	must(err)
	fmt.Printf("paired device=%q expires=%s\n", claimed.DeviceName, claimed.ExpiresAt)

	wsURL, err := mobileWebSocketURL(relayURL)
	must(err)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + claimed.Token}},
	})
	must(err)
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(8 << 20)

	hello := readMessage(ctx, conn)
	fmt.Printf("hello pairedSessions=%d\n", hello.PairedSessions)

	status := rpc(ctx, conn, "magpie.admin.status", nil)
	printSafe("magpie.admin.status", status)
	must(rpcOK("magpie.admin.status", status))
	var admin struct {
		Ready bool `json:"ready"`
	}
	must(json.Unmarshal(status.Result, &admin))
	if !admin.Ready {
		initialized := rpc(ctx, conn, "magpie.admin.initialize", nil)
		printSafe("magpie.admin.initialize", initialized)
		must(rpcOK("magpie.admin.initialize", initialized))
		status = rpc(ctx, conn, "magpie.admin.status", nil)
		printSafe("magpie.admin.status(after initialize)", status)
		must(rpcOK("magpie.admin.status(after initialize)", status))
		must(json.Unmarshal(status.Result, &admin))
		if !admin.Ready {
			fmt.Fprintln(os.Stderr, "Magpie admin is still not ready after initialize")
			fail()
		}
	}

	name := fmt.Sprintf("codexlink-probe-%d", time.Now().Unix())
	created := rpc(ctx, conn, "magpie.keys.create", map[string]any{
		"name":     name,
		"readOnly": true,
	})
	printSafe("magpie.keys.create", created)
	if !created.OK {
		fail()
	}
	var secret struct {
		KeyID   string `json:"keyId"`
		KeyName string `json:"keyName"`
	}
	must(json.Unmarshal(created.Result, &secret))
	if strings.TrimSpace(secret.KeyID) == "" {
		fmt.Fprintln(os.Stderr, "create returned an empty keyId")
		fail()
	}
	fmt.Printf("created keyId=%q name=%q (secret intentionally not printed)\n", secret.KeyID, secret.KeyName)
	defer func() {
		if secret.KeyID == "" {
			return
		}
		removed := rpc(ctx, conn, "magpie.keys.delete", map[string]any{"keyId": secret.KeyID})
		printSafe("magpie.keys.delete(cleanup)", removed)
	}()

	rotated := rpc(ctx, conn, "magpie.keys.rotate", map[string]any{"keyId": secret.KeyID})
	printSafe("magpie.keys.rotate", rotated)
	must(rpcOK("magpie.keys.rotate", rotated))

	disabled := rpc(ctx, conn, "magpie.keys.toggle", map[string]any{"keyId": secret.KeyID, "enabled": false})
	printSafe("magpie.keys.toggle(false)", disabled)
	must(rpcOK("magpie.keys.toggle(false)", disabled))

	enabled := rpc(ctx, conn, "magpie.keys.toggle", map[string]any{"keyId": secret.KeyID, "enabled": true})
	printSafe("magpie.keys.toggle(true)", enabled)
	must(rpcOK("magpie.keys.toggle(true)", enabled))

	removed := rpc(ctx, conn, "magpie.keys.delete", map[string]any{"keyId": secret.KeyID})
	printSafe("magpie.keys.delete", removed)
	must(rpcOK("magpie.keys.delete", removed))
	secret.KeyID = ""

	finalStatus := rpc(ctx, conn, "magpie.admin.status", nil)
	printSafe("magpie.admin.status(final)", finalStatus)
	must(rpcOK("magpie.admin.status(final)", finalStatus))

	fmt.Println("RESULT PASS")
}

func claimPair(ctx context.Context, relayURL, code, name string) (relayproto.PairClaimResponse, error) {
	body, _ := json.Marshal(relayproto.PairClaimRequest{Code: code, Name: name})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, relayURL+relayproto.PathPairClaim, bytes.NewReader(body))
	if err != nil {
		return relayproto.PairClaimResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return relayproto.PairClaimResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return relayproto.PairClaimResponse{}, fmt.Errorf("pair claim status=%d body=%s", resp.StatusCode, raw)
	}
	var result relayproto.PairClaimResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return relayproto.PairClaimResponse{}, err
	}
	return result, nil
}

func mobileWebSocketURL(relayURL string) (string, error) {
	parsed, err := url.Parse(relayURL)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", fmt.Errorf("unsupported relay scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + relayproto.PathMobileConnect
	return parsed.String(), nil
}

func rpc(ctx context.Context, conn *websocket.Conn, method string, params any) relayproto.Message {
	requestID := "keyprobe-" + fmt.Sprint(time.Now().UnixNano())
	message := relayproto.Message{Type: relayproto.MessageTypeRPC, RequestID: requestID, Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		must(err)
		message.Params = raw
	}
	raw, err := json.Marshal(message)
	must(err)
	must(conn.Write(ctx, websocket.MessageText, raw))
	for {
		result := readMessage(ctx, conn)
		if result.Type == relayproto.MessageTypeRPCResult && result.RequestID == requestID {
			return result
		}
	}
}

func readMessage(ctx context.Context, conn *websocket.Conn) relayproto.Message {
	_, raw, err := conn.Read(ctx)
	must(err)
	var message relayproto.Message
	must(json.Unmarshal(raw, &message))
	return message
}

func printSafe(name string, message relayproto.Message) {
	if !message.OK {
		fmt.Printf("%s ok=false error=%s\n", name, message.Error)
		return
	}
	fmt.Printf("%s ok=true result=%s\n", name, safeResult(message.Result))
}

func safeResult(raw json.RawMessage) string {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err == nil {
		for _, key := range []string{"secret", "token", "adminKey", "apiKey"} {
			if _, ok := value[key]; ok {
				value[key] = "[redacted]"
			}
		}
		if encoded, err := json.Marshal(value); err == nil {
			return truncate(string(encoded), 700)
		}
	}
	return truncate(string(raw), 700)
}

func rpcOK(name string, message relayproto.Message) error {
	if message.OK {
		return nil
	}
	return fmt.Errorf("%s failed: %s", name, message.Error)
}

func truncate(value string, max int) string {
	if len(value) > max {
		return value[:max] + "..."
	}
	return value
}

func fail() {
	os.Exit(1)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
