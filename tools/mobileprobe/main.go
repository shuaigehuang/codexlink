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
		fmt.Fprintln(os.Stderr, "usage: mobileprobe <relay-url> <pair-code> [phone-name]")
		os.Exit(2)
	}
	relayURL := strings.TrimRight(os.Args[1], "/")
	pairCode := strings.ReplaceAll(os.Args[2], " ", "")
	phoneName := "CodexLink Test Phone"
	if len(os.Args) > 3 {
		phoneName = os.Args[3]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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

	ping := rpc(ctx, conn, "ping", nil)
	fmt.Printf("ping ok=%v result=%s\n", ping.OK, compact(ping.Result))

	status := rpc(ctx, conn, "node.status", nil)
	printResult("node.status", status)

	threads := rpc(ctx, conn, "threads.list", map[string]any{"limit": 5})
	printResult("threads.list", threads)
	if !threads.OK {
		return
	}
	var list []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(threads.Result, &list); err == nil && len(list) > 0 {
		detail := rpc(ctx, conn, "threads.read", map[string]any{"threadId": list[0].ID})
		printResult("threads.read", detail)
	}

	magpie := rpc(ctx, conn, "magpie.status", nil)
	printResult("magpie.status", magpie)
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
	requestID := "probe-" + method + "-" + fmt.Sprint(time.Now().UnixNano())
	message := relayproto.Message{Type: relayproto.MessageTypeRPC, RequestID: requestID, Method: method}
	if params != nil {
		raw, err := json.Marshal(params)
		must(err)
		message.Params = raw
	}
	writeMessage(ctx, conn, message)
	for {
		result := readMessage(ctx, conn)
		if result.Type == relayproto.MessageTypeRPCResult && result.RequestID == requestID {
			return result
		}
	}
}

func writeMessage(ctx context.Context, conn *websocket.Conn, message relayproto.Message) {
	raw, err := json.Marshal(message)
	must(err)
	must(conn.Write(ctx, websocket.MessageText, raw))
}

func readMessage(ctx context.Context, conn *websocket.Conn) relayproto.Message {
	_, raw, err := conn.Read(ctx)
	must(err)
	var message relayproto.Message
	must(json.Unmarshal(raw, &message))
	return message
}

func printResult(name string, message relayproto.Message) {
	if !message.OK {
		fmt.Printf("%s ok=false error=%s\n", name, message.Error)
		return
	}
	fmt.Printf("%s ok=true result=%s\n", name, compact(message.Result))
}

func compact(raw json.RawMessage) string {
	const max = 500
	value := string(raw)
	if len(value) > max {
		return value[:max] + "..."
	}
	return value
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
