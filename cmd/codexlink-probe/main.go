package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shuaigehuang/codexlink/internal/codex"
	"github.com/shuaigehuang/codexlink/internal/magpie"
)

type threadSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Preview   string `json:"preview"`
	Cwd       string `json:"cwd"`
	Model     string `json:"model"`
	Provider  string `json:"modelProvider"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

type turnSummary struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	ItemCount int      `json:"itemCount"`
	ItemTypes []string `json:"itemTypes"`
}

type sendTestResult struct {
	OK         bool     `json:"ok"`
	ThreadID   string   `json:"threadId"`
	TurnID     string   `json:"turnId"`
	Status     string   `json:"status"`
	ItemTypes  []string `json:"itemTypes"`
	AgentReply string   `json:"agentReply"`
	ElapsedMS  int64    `json:"elapsedMs"`
}

type output struct {
	OK         bool                     `json:"ok"`
	Server     codex.InitializeResponse `json:"server"`
	Threads    []threadSummary          `json:"threads"`
	Latest     threadSummary            `json:"latestThread"`
	Turns      []turnSummary            `json:"turns"`
	ItemTypes  []string                 `json:"itemTypes"`
	ItemCount  int                      `json:"itemCount"`
	SendTest   any                      `json:"sendTest"`
	Magpie     map[string]any           `json:"magpie"`
	CodexError string                   `json:"codexError,omitempty"`
}

func main() {
	codexPath := flag.String("codex", "", "path to codex executable")
	magpiePath := flag.String("magpie", "", "path to magpie.com")
	sendTest := flag.Bool("send-test", false, "send a harmless prompt to an ephemeral thread")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	result := output{
		OK:     true,
		Magpie: make(map[string]any),
	}

	client := codex.New(codex.Options{Executable: *codexPath})
	if err := client.Start(ctx); err != nil {
		result.OK = false
		result.CodexError = err.Error()
		printOutput(result)
		os.Exit(1)
	}
	defer client.Close()

	server, err := client.Initialize(ctx)
	if err != nil {
		result.OK = false
		result.CodexError = err.Error()
		printOutput(result)
		os.Exit(1)
	}
	result.Server = server

	threads, err := client.ListThreads(ctx, codex.ThreadListParams{
		Limit:         10,
		SortKey:       "updated_at",
		SortDirection: "desc",
	})
	if err != nil {
		result.OK = false
		result.CodexError = err.Error()
		printOutput(result)
		os.Exit(1)
	}

	for _, thread := range threads.Data {
		result.Threads = append(result.Threads, summarizeThread(thread))
	}
	if len(threads.Data) == 0 {
		result.OK = false
		result.CodexError = "thread/list returned no threads"
		printOutput(result)
		os.Exit(1)
	}

	latest := threads.Data[0]
	result.Latest = summarizeThread(latest)

	turns, err := client.ListTurns(ctx, codex.ThreadTurnsListParams{
		ThreadID:      latest.ID,
		Limit:         3,
		SortDirection: "desc",
		ItemsView:     "summary",
	})
	if err != nil {
		result.OK = false
		result.CodexError = err.Error()
		printOutput(result)
		os.Exit(1)
	}

	for _, turn := range turns.Data {
		result.Turns = append(result.Turns, summarizeTurn(turn))
	}

	if len(turns.Data) > 0 {
		items, err := client.ListItems(ctx, codex.ThreadItemsListParams{
			ThreadID:      latest.ID,
			TurnID:        turns.Data[0].ID,
			Limit:         20,
			SortDirection: "asc",
		})
		if err == nil {
			result.ItemCount = len(items.Data)
			seen := make(map[string]bool)
			for _, entry := range items.Data {
				if entry.Item.Type != "" && !seen[entry.Item.Type] {
					seen[entry.Item.Type] = true
					result.ItemTypes = append(result.ItemTypes, entry.Item.Type)
				}
			}
		}
	}

	if *sendTest {
		sendResult, err := runSendTest(ctx, client)
		if err != nil {
			result.OK = false
			result.CodexError = err.Error()
		} else {
			result.SendTest = sendResult
		}
	} else {
		result.SendTest = map[string]bool{"skipped": true}
	}

	readMagpie(ctx, *magpiePath, result.Magpie)
	printOutput(result)
}

func runSendTest(ctx context.Context, client *codex.Client) (sendTestResult, error) {
	cwd := os.TempDir()
	started, err := client.StartThread(ctx, codex.ThreadStartParams{
		Ephemeral:      true,
		Cwd:            cwd,
		ApprovalPolicy: "never",
		Sandbox:        "read-only",
		ThreadSource:   "codexlink-probe",
	})
	if err != nil {
		return sendTestResult{}, err
	}

	startedAt := time.Now()
	turn, err := client.StartTurn(ctx, codex.TurnStartParams{
		ThreadID:       started.Thread.ID,
		ApprovalPolicy: "never",
		Input: []codex.UserInput{
			{
				Type: "text",
				Text: "只回复 OK，不要调用任何工具，不要执行任何命令。",
			},
		},
	})
	if err != nil {
		return sendTestResult{}, err
	}

	completed, err := client.WaitForTurnCompleted(ctx, started.Thread.ID, turn.Turn.ID)
	if err != nil {
		return sendTestResult{}, err
	}

	var itemTypes []string
	var agentReply string
	var streamed strings.Builder
	seenTypes := make(map[string]bool)
	for _, notification := range client.Notifications() {
		switch notification.Method {
		case "item/completed":
			var params codex.ItemCompletedParams
			if json.Unmarshal(notification.Params, &params) != nil {
				continue
			}
			if params.ThreadID != started.Thread.ID || params.TurnID != turn.Turn.ID {
				continue
			}
			if params.Item.Type != "" && !seenTypes[params.Item.Type] {
				seenTypes[params.Item.Type] = true
				itemTypes = append(itemTypes, params.Item.Type)
			}
			if params.Item.Type == "agentMessage" && params.Item.Text != "" {
				agentReply = params.Item.Text
			}
		case "item/agentMessage/delta":
			var params codex.AgentMessageDeltaParams
			if json.Unmarshal(notification.Params, &params) != nil {
				continue
			}
			if params.ThreadID == started.Thread.ID && params.TurnID == turn.Turn.ID {
				streamed.WriteString(params.Delta)
			}
		}
	}
	if agentReply == "" {
		agentReply = streamed.String()
	}

	return sendTestResult{
		OK:         true,
		ThreadID:   started.Thread.ID,
		TurnID:     turn.Turn.ID,
		Status:     completed.Turn.Status,
		ItemTypes:  itemTypes,
		AgentReply: agentReply,
		ElapsedMS:  time.Since(startedAt).Milliseconds(),
	}, nil
}

func readMagpie(ctx context.Context, consolePath string, result map[string]any) {
	if consolePath == "" {
		path, err := magpie.FindConsole()
		if err != nil {
			result["ok"] = false
			result["error"] = err.Error()
			return
		}
		consolePath = path
	}

	client := magpie.New(consolePath)
	status, err := client.Status(ctx)
	if err != nil {
		result["ok"] = false
		result["error"] = err.Error()
		return
	}
	result["ok"] = true
	result["status"] = status

	quota, err := client.Quota(ctx)
	if err == nil {
		result["quota"] = quota
	}
	providers, err := client.Providers(ctx)
	if err == nil {
		result["providers"] = providers
	}
	gatewayKeys, err := client.GatewayKeys(ctx)
	if err == nil {
		result["gatewayKeys"] = gatewayKeys
	}
}

func summarizeThread(thread codex.Thread) threadSummary {
	return threadSummary{
		ID:        thread.ID,
		Name:      thread.Name,
		Preview:   thread.Preview,
		Cwd:       thread.Cwd,
		Model:     thread.Model,
		Provider:  thread.ModelProvider,
		CreatedAt: thread.CreatedAt,
		UpdatedAt: thread.UpdatedAt,
	}
}

func summarizeTurn(turn codex.Turn) turnSummary {
	seen := make(map[string]bool)
	var types []string
	for _, item := range turn.Items {
		if item.Type != "" && !seen[item.Type] {
			seen[item.Type] = true
			types = append(types, item.Type)
		}
	}
	return turnSummary{
		ID:        turn.ID,
		Status:    turn.Status,
		ItemCount: len(turn.Items),
		ItemTypes: types,
	}
}

func printOutput(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
