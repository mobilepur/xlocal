package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// cliTimeout bounds a single CLI invocation; starting an agent process is
// much slower than an API call.
const cliTimeout = 3 * time.Minute

// runner executes a binary with the prompt on stdin. It is a variable on the
// backends so tests can replace it.
type runner func(ctx context.Context, name string, args []string, stdin string) (stdout []byte, err error)

func execRunner(ctx context.Context, name string, args []string, stdin string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	// Run from a neutral directory so the CLI does not pick up a project's
	// CLAUDE.md, AGENTS.md or local settings.
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s failed: %w: %s", name, err, tail(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	const max = 400
	if len(s) > max {
		return "…" + s[len(s)-max:]
	}
	return s
}

// Claude translates through `claude -p` using the Claude subscription login.
type Claude struct {
	Binary string // resolved path of the claude binary
	Model  string // alias ("sonnet") or full ID; empty = CLI default
	run    runner
}

// NewClaude looks up the claude binary.
func NewClaude(model string) (*Claude, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("the claude CLI was not found in PATH — install Claude Code and sign in with `claude`")
	}
	return &Claude{Binary: path, Model: model, run: execRunner}, nil
}

func (c *Claude) args() []string {
	// No tools, no persisted session, no user/project settings: a plain
	// text-in/text-out call. --bare would also skip the subscription login.
	args := []string{"-p", "--output-format", "json", "--tools", "", "--no-session-persistence", "--setting-sources", ""}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	return args
}

type claudeResult struct {
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

func (c *Claude) Complete(ctx context.Context, prompt string) (string, error) {
	out, err := c.run(ctx, c.Binary, c.args(), prompt)
	if err != nil {
		return "", err
	}
	var res claudeResult
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("unexpected claude output: %w", err)
	}
	text := strings.TrimSpace(res.Result)
	if res.IsError {
		return "", fmt.Errorf("claude: %s", text)
	}
	if text == "" {
		return "", fmt.Errorf("no text in response")
	}
	return text, nil
}

// Codex translates through `codex exec` using the ChatGPT subscription login.
type Codex struct {
	Binary string
	Model  string // empty = Codex default
	run    runner
}

// NewCodex looks up the codex binary.
func NewCodex(model string) (*Codex, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("the codex CLI was not found in PATH — install Codex and sign in with `codex login`")
	}
	return &Codex{Binary: path, Model: model, run: execRunner}, nil
}

func (c *Codex) args(outFile string) []string {
	// --ignore-user-config keeps the user's MCP servers and model choice out
	// of the call; auth still comes from CODEX_HOME.
	args := []string{"exec", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "--ephemeral", "-s", "read-only", "-o", outFile}
	if c.Model != "" {
		args = append(args, "-m", c.Model)
	}
	return append(args, "-")
}

func (c *Codex) Complete(ctx context.Context, prompt string) (string, error) {
	f, err := os.CreateTemp("", "xlocal-codex-*.txt")
	if err != nil {
		return "", err
	}
	f.Close()
	defer os.Remove(f.Name())

	if _, err := c.run(ctx, c.Binary, c.args(f.Name()), prompt); err != nil {
		return "", err
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("no text in response")
	}
	return text, nil
}
