package relay

import (
	"encoding/json"
	"time"
)

const (
	PathHealth           = "/healthz"
	PathDesktopConnect   = "/v1/desktop/connect"
	PathDesktopSessions  = "/v1/desktop/sessions"
	PathDesktopRevoke    = "/v1/desktop/sessions/revoke"
	PathMobileConnect    = "/v1/mobile/connect"
	PathMobileRPC        = "/v1/mobile/rpc"
	PathPairClaim        = "/v1/pair/claim"
	MessageTypeHello     = "hello"
	MessageTypeError     = "error"
	MessageTypePair      = "pair.create"
	MessageTypePaired    = "pair.created"
	MessageTypeSessions  = "sessions.updated"
	MessageTypeRPC       = "rpc"
	MessageTypeRPCResult = "rpc.result"
	ErrorDesktopOffline  = "desktop_offline"
	ErrorUnauthorized    = "unauthorized"
	ErrorBadRequest      = "bad_request"
	ErrorMethodBlocked   = "method_not_allowed"
)

var allowedMobileMethods = map[string]bool{
	"ping":                    true,
	"node.status":             true,
	"threads.list":            true,
	"threads.read":            true,
	"threads.send":            true,
	"magpie.status":           true,
	"magpie.admin.status":     true,
	"magpie.admin.initialize": true,
	"magpie.keys.create":      true,
	"magpie.keys.rotate":      true,
	"magpie.keys.toggle":      true,
	"magpie.keys.delete":      true,
}

func isAllowedMobileMethod(method string) bool {
	return allowedMobileMethods[method]
}

type Message struct {
	Type           string          `json:"type"`
	RequestID      string          `json:"requestId,omitempty"`
	Method         string          `json:"method,omitempty"`
	Params         json.RawMessage `json:"params,omitempty"`
	OK             bool            `json:"ok,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	Code           string          `json:"code,omitempty"`
	ExpiresAt      string          `json:"expiresAt,omitempty"`
	DeviceID       string          `json:"deviceId,omitempty"`
	DeviceName     string          `json:"deviceName,omitempty"`
	PairedSessions int             `json:"pairedSessions,omitempty"`
}

type PairClaimRequest struct {
	Code string `json:"code"`
	Name string `json:"name,omitempty"`
}

type PairClaimResponse struct {
	Token      string `json:"token"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName,omitempty"`
	ExpiresAt  string `json:"expiresAt"`
}

type Pairing struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

type MobileSession struct {
	ID         string `json:"id"`
	DeviceName string `json:"deviceName,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	ExpiresAt  string `json:"expiresAt"`
	Active     bool   `json:"active"`
}

type MobileSessionList struct {
	Sessions []MobileSession `json:"sessions"`
	Count    int             `json:"count"`
}

type RevokeSessionsRequest struct {
	DeviceID  string `json:"deviceId"`
	SessionID string `json:"sessionId,omitempty"`
	All       bool   `json:"all,omitempty"`
}

type RevokeSessionsResponse struct {
	Revoked int `json:"revoked"`
}

func rpcResultMessage(requestID string, ok bool, result any, errMessage string) Message {
	message := Message{
		Type:      MessageTypeRPCResult,
		RequestID: requestID,
		OK:        ok,
		Error:     errMessage,
	}
	if ok && result != nil {
		if encoded, err := json.Marshal(result); err == nil {
			message.Result = encoded
		} else {
			message.OK = false
			message.Error = err.Error()
		}
	}
	return message
}

func expiresAt(now time.Time, ttl time.Duration) string {
	return now.Add(ttl).UTC().Format(time.RFC3339)
}
