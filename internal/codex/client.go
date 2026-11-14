package codex

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Options struct {
	Executable string
	Args       []string
	Env        []string
	Dir        string
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("codex rpc error %d: %s", e.Code, e.Message)
}

type Notification struct {
	Method string
	Params json.RawMessage
}

type Client struct {
	opts Options

	cmd   *exec.Cmd
	stdin io.WriteCloser
	path  string

	writeMu sync.Mutex

	mu            sync.Mutex
	pending       map[int64]chan callResult
	waiters       map[int64]*notificationWaiter
	nextWaiterID  int64
	notifications []Notification
	closed        bool
	closeErr      error
	nextID        atomic.Int64

	stderrMu sync.Mutex
	stderr   strings.Builder

	done      chan struct{}
	closeOnce sync.Once
}

type callResult struct {
	result json.RawMessage
	err    error
}

type notificationWaiter struct {
	method string
	match  func(Notification) bool
	ch     chan Notification
}

type wireMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type rpcRequest struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type rpcNotification struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

func New(opts Options) *Client {
	return &Client{
		opts:    opts,
		pending: make(map[int64]chan callResult),
		waiters: make(map[int64]*notificationWaiter),
		done:    make(chan struct{}),
	}
}

func (c *Client) Start(ctx context.Context) error {
	executable, err := c.resolveExecutable()
	if err != nil {
		return err
	}
	c.path = executable

	args := []string{"app-server", "--listen", "stdio://"}
	args = append(args, c.opts.Args...)

	cmd := exec.CommandContext(ctx, executable, args...)
	configureProcess(cmd)
	if c.opts.Dir != "" {
		cmd.Dir = c.opts.Dir
	}
	if len(c.opts.Env) > 0 {
		cmd.Env = append(os.Environ(), c.opts.Env...)
	}

	cmd.Stderr = &lockedWriter{client: c}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open app-server stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start app-server: %w", err)
	}

	c.cmd = cmd
	c.stdin = stdin

	go c.readLoop(stdout)
	go c.waitLoop()
	return nil
}

func (c *Client) Executable() string {
	return c.path
}

func (c *Client) resolveExecutable() (string, error) {
	candidates := []string{
		c.opts.Executable,
		os.Getenv("CODEX_CLI_PATH"),
	}
	candidates = append(candidates, platformExecutableCandidates()...)
	candidates = append(candidates, "codex.exe", "codex")
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("codex executable not found")
}

func (c *Client) Initialize(ctx context.Context) (InitializeResponse, error) {
	var response InitializeResponse
	err := c.Call(ctx, "initialize", InitializeParams{
		ClientInfo: ClientInfo{
			Name:    "codexlink",
			Title:   "CodexLink",
			Version: "1.0.8",
		},
		Capabilities: InitializeCapabilities{
			ExperimentalAPI: true,
		},
	}, &response)
	if err != nil {
		return response, err
	}
	if err := c.Notify("initialized", nil); err != nil {
		return response, err
	}
	return response, nil
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	raw, err := c.callRaw(ctx, method, params)
	if err != nil {
		return err
	}
	if result == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	return nil
}

func (c *Client) callRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		err := c.closeErr
		c.mu.Unlock()
		if err == nil {
			err = io.ErrClosedPipe
		}
		return nil, err
	}
	id := c.nextID.Add(1)
	ch := make(chan callResult, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.writeJSON(rpcRequest{ID: id, Method: method, Params: params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case response := <-ch:
		return response.result, response.err
	}
}

func (c *Client) Notify(method string, params any) error {
	return c.writeJSON(rpcNotification{Method: method, Params: params})
}

func (c *Client) writeJSON(value any) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return io.ErrClosedPipe
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write app-server request: %w", err)
	}
	return nil
}

func (c *Client) readLoop(stdout io.Reader) {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) == 0 {
			if readErr == nil {
				continue
			}
			if readErr == io.EOF {
				c.failAll(io.EOF)
				return
			}
			c.failAll(fmt.Errorf("read app-server output: %w", readErr))
			return
		}

		var message wireMessage
		if err := json.Unmarshal(line, &message); err != nil {
			if readErr == nil {
				continue
			}
			if readErr == io.EOF {
				c.failAll(io.EOF)
				return
			}
			c.failAll(fmt.Errorf("read app-server output: %w", readErr))
			return
		}

		if message.Method != "" {
			c.dispatchNotification(Notification{
				Method: message.Method,
				Params: message.Params,
			})
		} else {
			id, err := parseMessageID(message.ID)
			if err == nil {
				c.mu.Lock()
				ch := c.pending[id]
				delete(c.pending, id)
				c.mu.Unlock()
				if ch != nil {
					if message.Error != nil {
						ch <- callResult{err: message.Error}
					} else {
						ch <- callResult{result: message.Result}
					}
				}
			}
		}

		if readErr == io.EOF {
			c.failAll(io.EOF)
			return
		}
		if readErr != nil {
			c.failAll(fmt.Errorf("read app-server output: %w", readErr))
			return
		}
	}
}

func (c *Client) waitLoop() {
	err := c.cmd.Wait()
	if err == nil {
		err = io.EOF
	}
	c.failAll(fmt.Errorf("app-server stopped: %w", err))
	c.closeOnce.Do(func() {
		close(c.done)
	})
}

func (c *Client) failAll(err error) {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.closeErr = err
	}
	pending := c.pending
	c.pending = make(map[int64]chan callResult)
	waiters := c.waiters
	c.waiters = make(map[int64]*notificationWaiter)
	c.mu.Unlock()

	for _, ch := range pending {
		select {
		case ch <- callResult{err: err}:
		default:
		}
	}
	for _, waiter := range waiters {
		close(waiter.ch)
	}
}

func (c *Client) dispatchNotification(notification Notification) {
	c.mu.Lock()
	c.notifications = append(c.notifications, notification)
	if len(c.notifications) > 5000 {
		c.notifications = append([]Notification(nil), c.notifications[len(c.notifications)-4000:]...)
	}
	waiters := make([]*notificationWaiter, 0, len(c.waiters))
	for id, waiter := range c.waiters {
		if waiter.method != notification.Method {
			continue
		}
		if waiter.match != nil && !waiter.match(notification) {
			continue
		}
		delete(c.waiters, id)
		waiters = append(waiters, waiter)
	}
	c.mu.Unlock()

	for _, waiter := range waiters {
		select {
		case waiter.ch <- notification:
		default:
		}
		close(waiter.ch)
	}
}

func (c *Client) WaitForNotification(
	ctx context.Context,
	method string,
	match func(Notification) bool,
) (Notification, error) {
	c.mu.Lock()
	if c.closed && len(c.notifications) == 0 {
		err := c.closeErr
		c.mu.Unlock()
		if err == nil {
			err = io.ErrClosedPipe
		}
		return Notification{}, err
	}
	for _, notification := range c.notifications {
		if notification.Method != method {
			continue
		}
		if match == nil || match(notification) {
			c.mu.Unlock()
			return notification, nil
		}
	}
	if c.closed {
		err := c.closeErr
		c.mu.Unlock()
		if err == nil {
			err = io.ErrClosedPipe
		}
		return Notification{}, err
	}

	waiterID := atomic.AddInt64(&c.nextWaiterID, 1)
	waiter := &notificationWaiter{
		method: method,
		match:  match,
		ch:     make(chan Notification, 1),
	}
	c.waiters[waiterID] = waiter
	c.mu.Unlock()

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.waiters, waiterID)
		c.mu.Unlock()
		return Notification{}, ctx.Err()
	case notification, ok := <-waiter.ch:
		if !ok {
			return Notification{}, io.EOF
		}
		return notification, nil
	}
}

func (c *Client) Notifications() []Notification {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Notification(nil), c.notifications...)
}

func (c *Client) ListThreads(ctx context.Context, params ThreadListParams) (ThreadListResponse, error) {
	var response ThreadListResponse
	err := c.Call(ctx, "thread/list", params, &response)
	return response, err
}

func (c *Client) ReadThread(ctx context.Context, params ThreadReadParams) (ThreadReadResponse, error) {
	var response ThreadReadResponse
	err := c.Call(ctx, "thread/read", params, &response)
	return response, err
}

func (c *Client) ResumeThread(ctx context.Context, params ThreadResumeParams) (ThreadResumeResponse, error) {
	var response ThreadResumeResponse
	err := c.Call(ctx, "thread/resume", params, &response)
	return response, err
}

func (c *Client) ListTurns(ctx context.Context, params ThreadTurnsListParams) (ThreadTurnsListResponse, error) {
	var response ThreadTurnsListResponse
	err := c.Call(ctx, "thread/turns/list", params, &response)
	return response, err
}

func (c *Client) ListItems(ctx context.Context, params ThreadItemsListParams) (ThreadItemsListResponse, error) {
	var response ThreadItemsListResponse
	err := c.Call(ctx, "thread/items/list", params, &response)
	return response, err
}

func (c *Client) StartThread(ctx context.Context, params ThreadStartParams) (ThreadStartResponse, error) {
	var response ThreadStartResponse
	err := c.Call(ctx, "thread/start", params, &response)
	return response, err
}

func (c *Client) StartTurn(ctx context.Context, params TurnStartParams) (TurnStartResponse, error) {
	var response TurnStartResponse
	err := c.Call(ctx, "turn/start", params, &response)
	return response, err
}

func (c *Client) QueueThreadInput(ctx context.Context, threadID, text string) (QueuedSubmission, error) {
	var response ThreadQueueAddResponse
	err := c.Call(ctx, "thread/queue/add", ThreadQueueAddParams{
		ThreadID:            threadID,
		ClientUserMessageID: newClientMessageID(),
		Input: []UserInput{
			{Type: "text", Text: text},
		},
	}, &response)
	return response.QueuedSubmission, err
}

func (c *Client) DeleteQueuedInput(ctx context.Context, threadID, queuedSubmissionID string) (bool, error) {
	var response ThreadQueueDeleteResponse
	err := c.Call(ctx, "thread/queue/delete", ThreadQueueDeleteParams{
		ThreadID:           threadID,
		QueuedSubmissionID: queuedSubmissionID,
	}, &response)
	return response.Deleted, err
}

func (c *Client) InterruptTurn(ctx context.Context, params InterruptTurnParams) error {
	return c.Call(ctx, "turn/interrupt", params, nil)
}

func newClientMessageID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(value[:])
}

func (c *Client) WaitForTurnCompleted(ctx context.Context, threadID, turnID string) (TurnCompletedParams, error) {
	notification, err := c.WaitForNotification(ctx, "turn/completed", func(notification Notification) bool {
		var params TurnCompletedParams
		if err := json.Unmarshal(notification.Params, &params); err != nil {
			return false
		}
		if params.ThreadID != threadID {
			return false
		}
		return turnID == "" || params.Turn.ID == turnID
	})
	if err != nil {
		return TurnCompletedParams{}, err
	}

	var params TurnCompletedParams
	err = json.Unmarshal(notification.Params, &params)
	return params, err
}

func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	select {
	case <-c.done:
		return nil
	case <-time.After(3 * time.Second):
		return c.cmd.Process.Kill()
	}
}

func (c *Client) StderrTail() string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	return c.stderr.String()
}

func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		return nil
	}
	if c.closeErr != nil {
		return c.closeErr
	}
	return io.ErrClosedPipe
}

func (c *Client) appendStderr(text string) {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	c.stderr.WriteString(text)
	if c.stderr.Len() > 16000 {
		current := c.stderr.String()
		c.stderr.Reset()
		c.stderr.WriteString(current[len(current)-12000:])
	}
}

type lockedWriter struct {
	client *Client
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.client.appendStderr(string(data))
	return len(data), nil
}

func parseMessageID(raw json.RawMessage) (int64, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, errors.New("missing message id")
	}
	text = strings.Trim(text, `"`)
	return strconv.ParseInt(text, 10, 64)
}
