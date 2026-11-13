package codex

import (
	"bytes"
	"encoding/json"
)

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

type InitializeParams struct {
	ClientInfo   ClientInfo `json:"clientInfo"`
	Capabilities any        `json:"capabilities,omitempty"`
}

type InitializeCapabilities struct {
	ExperimentalAPI bool `json:"experimentalApi,omitempty"`
}

type InitializeResponse struct {
	UserAgent      string `json:"userAgent"`
	CodexHome      string `json:"codexHome"`
	PlatformFamily string `json:"platformFamily"`
	PlatformOS     string `json:"platformOs"`
}

type ThreadListParams struct {
	Limit         int    `json:"limit,omitempty"`
	SortKey       string `json:"sortKey,omitempty"`
	SortDirection string `json:"sortDirection,omitempty"`
	SearchTerm    string `json:"searchTerm,omitempty"`
	Archived      *bool  `json:"archived,omitempty"`
}

type Thread struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Preview         string          `json:"preview"`
	Cwd             string          `json:"cwd"`
	Model           string          `json:"model"`
	ModelProvider   string          `json:"modelProvider"`
	ReasoningEffort string          `json:"reasoningEffort"`
	CreatedAt       int64           `json:"createdAt"`
	UpdatedAt       int64           `json:"updatedAt"`
	RecencyAt       *int64          `json:"recencyAt"`
	Source          json.RawMessage `json:"source"`
	Status          json.RawMessage `json:"status"`
	Path            string          `json:"path"`
	SessionID       string          `json:"sessionId"`
	Ephemeral       bool            `json:"ephemeral"`
	CanAcceptDirect *bool           `json:"canAcceptDirectInput"`
	HistoryMode     string          `json:"historyMode"`
	ProjectID       string          `json:"projectId"`
	ForkedFromID    string          `json:"forkedFromId"`
	ParentThreadID  string          `json:"parentThreadId"`
	AgentNickname   string          `json:"agentNickname"`
	AgentRole       string          `json:"agentRole"`
	Turns           []Turn          `json:"turns"`
}

type ThreadListResponse struct {
	Data       []Thread `json:"data"`
	NextCursor string   `json:"nextCursor"`
}

type ThreadReadParams struct {
	ThreadID     string `json:"threadId"`
	IncludeTurns bool   `json:"includeTurns,omitempty"`
}

type ThreadReadResponse struct {
	Thread    Thread          `json:"thread"`
	ReadState json.RawMessage `json:"readState"`
}

type ThreadResumeParams struct {
	ThreadID       string `json:"threadId"`
	ExcludeTurns   bool   `json:"excludeTurns,omitempty"`
	Cwd            string `json:"cwd,omitempty"`
	Model          string `json:"model,omitempty"`
	ModelProvider  string `json:"modelProvider,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	Sandbox        string `json:"sandbox,omitempty"`
}

type ThreadResumeResponse struct {
	Thread Thread `json:"thread"`
}

type ThreadTurnsListParams struct {
	ThreadID      string `json:"threadId"`
	Limit         int    `json:"limit,omitempty"`
	SortDirection string `json:"sortDirection,omitempty"`
	ItemsView     string `json:"itemsView,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
}

type ThreadTurnsListResponse struct {
	Data       []Turn `json:"data"`
	NextCursor string `json:"nextCursor"`
}

type ThreadItemsListParams struct {
	ThreadID      string `json:"threadId"`
	TurnID        string `json:"turnId,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	SortDirection string `json:"sortDirection,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
}

type ThreadItemsListResponse struct {
	Data       []ThreadItemEntry `json:"data"`
	NextCursor string            `json:"nextCursor"`
}

type ThreadStartParams struct {
	Ephemeral      bool   `json:"ephemeral,omitempty"`
	Cwd            string `json:"cwd,omitempty"`
	ApprovalPolicy string `json:"approvalPolicy,omitempty"`
	Sandbox        string `json:"sandbox,omitempty"`
	Model          string `json:"model,omitempty"`
	ModelProvider  string `json:"modelProvider,omitempty"`
	ThreadSource   string `json:"threadSource,omitempty"`
}

type ThreadStartResponse struct {
	Thread Thread `json:"thread"`
}

type TurnStartParams struct {
	ThreadID       string      `json:"threadId"`
	Input          []UserInput `json:"input"`
	ApprovalPolicy string      `json:"approvalPolicy,omitempty"`
	Cwd            string      `json:"cwd,omitempty"`
	Model          string      `json:"model,omitempty"`
	Effort         string      `json:"effort,omitempty"`
}

type TurnStartResponse struct {
	Turn Turn `json:"turn"`
}

type ThreadQueueAddParams struct {
	ThreadID            string      `json:"threadId"`
	ClientUserMessageID string      `json:"clientUserMessageId"`
	Input               []UserInput `json:"input"`
}

type QueuedSubmission struct {
	ID                  string      `json:"id"`
	ClientUserMessageID string      `json:"clientUserMessageId"`
	Input               []UserInput `json:"input"`
}

type ThreadQueueAddResponse struct {
	QueuedSubmission QueuedSubmission `json:"queuedSubmission"`
}

type ThreadQueueDeleteParams struct {
	ThreadID           string `json:"threadId"`
	QueuedSubmissionID string `json:"queuedSubmissionId"`
}

type ThreadQueueDeleteResponse struct {
	Deleted bool `json:"deleted"`
}

type InterruptTurnParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type Turn struct {
	ID          string       `json:"id"`
	Status      string       `json:"status"`
	Items       []ThreadItem `json:"items"`
	StartedAt   *int64       `json:"startedAt"`
	CompletedAt *int64       `json:"completedAt"`
}

type UserInput struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (u *UserInput) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*u = UserInput{Type: "text", Text: text}
		return nil
	}

	type plainUserInput UserInput
	var decoded plainUserInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = UserInput(decoded)
	return nil
}

type ThreadItem struct {
	Type             string      `json:"type"`
	ID               string      `json:"id"`
	Text             string      `json:"text,omitempty"`
	Content          []UserInput `json:"content,omitempty"`
	Summary          []any       `json:"summary,omitempty"`
	Command          string      `json:"command,omitempty"`
	AggregatedOutput string      `json:"aggregatedOutput,omitempty"`
	StartedAtMs      *int64      `json:"startedAtMs,omitempty"`
	CompletedAtMs    *int64      `json:"completedAtMs,omitempty"`
}

type ThreadItemEntry struct {
	Item          ThreadItem `json:"item"`
	TurnID        string     `json:"turnId"`
	StartedAtMs   *int64     `json:"startedAtMs"`
	CompletedAtMs *int64     `json:"completedAtMs"`
}

type TurnCompletedParams struct {
	ThreadID string `json:"threadId"`
	Turn     Turn   `json:"turn"`
}

type AgentMessageDeltaParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

type ItemCompletedParams struct {
	ThreadID      string     `json:"threadId"`
	TurnID        string     `json:"turnId"`
	Item          ThreadItem `json:"item"`
	CompletedAtMs int64      `json:"completedAtMs"`
}
