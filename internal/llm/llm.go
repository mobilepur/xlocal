// Package llm abstracts the backends xlocal can translate with: the Anthropic
// API (API key) and the locally installed Claude Code and Codex CLIs, which
// use the user's own Claude or ChatGPT subscription.
package llm

import (
	"context"
	"fmt"
	"strings"
)

// Completer sends a single prompt to a model and returns its trimmed text.
type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Provider selects the backend.
type Provider string

const (
	// Anthropic calls the Anthropic API with an API key (the default).
	Anthropic Provider = "anthropic"
	// ClaudeCLI runs `claude -p` with the user's Claude subscription login.
	ClaudeCLI Provider = "claude-cli"
	// CodexCLI runs `codex exec` with the user's ChatGPT subscription login.
	CodexCLI Provider = "codex-cli"
)

// Providers lists all valid values in display order.
var Providers = []Provider{Anthropic, ClaudeCLI, CodexCLI}

// ParseProvider validates a provider name; empty means Anthropic.
func ParseProvider(s string) (Provider, error) {
	if s == "" {
		return Anthropic, nil
	}
	for _, p := range Providers {
		if string(p) == s {
			return p, nil
		}
	}
	names := make([]string, len(Providers))
	for i, p := range Providers {
		names[i] = string(p)
	}
	return "", fmt.Errorf("unknown provider %q (use %s)", s, strings.Join(names, ", "))
}

// NeedsAPIKey reports whether the provider authenticates with an API key.
func (p Provider) NeedsAPIKey() bool { return p == Anthropic }
