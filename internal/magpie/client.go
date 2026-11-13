package magpie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var secretPattern = regexp.MustCompile(`sk-[A-Za-z0-9._-]+`)

type Client struct {
	ConsolePath string
	Env         []string
	Timeout     time.Duration
}

type Status struct {
	Running bool   `json:"running"`
	URL     string `json:"url"`
	Addr    string `json:"addr"`
	Role    string `json:"role"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
	Started string `json:"started"`
	File    string `json:"file"`
}

type Quota struct {
	Provider     string `json:"provider"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Balance      string `json:"balance"`
	ReadAt       string `json:"readAt"`
	LastServedAt string `json:"lastServedAt"`
}

func New(consolePath string) *Client {
	return &Client{
		ConsolePath: consolePath,
		Timeout:     20 * time.Second,
	}
}

func FindConsole() (string, error) {
	if path := os.Getenv("MAGPIE_CONSOLE_PATH"); path != "" {
		return path, nil
	}
	if path, err := exec.LookPath("magpie.com"); err == nil {
		return path, nil
	}
	return "", errors.New("magpie.com not found; set MAGPIE_CONSOLE_PATH")
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	output, err := c.Run(ctx, "status", "--json")
	if err != nil {
		return Status{}, err
	}
	var status Status
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		return Status{}, fmt.Errorf("decode magpie status: %w", err)
	}
	return status, nil
}

func (c *Client) Quota(ctx context.Context) ([]Quota, error) {
	output, err := c.Run(ctx, "quota", "--json")
	if err != nil {
		return nil, err
	}
	var quota []Quota
	if err := json.Unmarshal([]byte(output), &quota); err != nil {
		return nil, fmt.Errorf("decode magpie quota: %w", err)
	}
	return quota, nil
}

func (c *Client) Providers(ctx context.Context) ([]string, error) {
	output, err := c.Run(ctx, "providers")
	if err != nil {
		return nil, err
	}
	return RedactLines(output), nil
}

func (c *Client) GatewayKeys(ctx context.Context) ([]string, error) {
	output, err := c.Run(ctx, "gateway-key", "list")
	if err != nil {
		return nil, err
	}
	return RedactLines(output), nil
}

func (c *Client) TestProvider(ctx context.Context, id string) (string, error) {
	return c.Run(ctx, "provider", "test", id)
}

func (c *Client) Run(ctx context.Context, args ...string) (string, error) {
	return c.run(ctx, true, args...)
}

// RunSecret runs a command whose stdout is a credential. The value is
// returned to the caller but is never included in an error or a log.
func (c *Client) RunSecret(ctx context.Context, args ...string) (string, error) {
	return c.run(ctx, false, args...)
}

func (c *Client) run(ctx context.Context, includeStdoutInError bool, args ...string) (string, error) {
	if c.ConsolePath == "" {
		path, err := FindConsole()
		if err != nil {
			return "", err
		}
		c.ConsolePath = path
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, c.ConsolePath, args...)
	configureProcess(cmd)
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if !includeStdoutInError {
			return "", fmt.Errorf(
				"magpie %s failed: %w; stderr=%s",
				strings.Join(args, " "),
				err,
				Redact(stderr.String()),
			)
		}
		return "", fmt.Errorf(
			"magpie %s failed: %w; stdout=%s; stderr=%s",
			strings.Join(args, " "),
			err,
			Redact(stdout.String()),
			Redact(stderr.String()),
		)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func Redact(text string) string {
	return secretPattern.ReplaceAllString(text, "sk-***")
}

func RedactLines(text string) []string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(Redact(line))
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
