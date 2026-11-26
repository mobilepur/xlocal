package llm

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseProvider(t *testing.T) {
	for in, want := range map[string]Provider{"": Anthropic, "anthropic": Anthropic, "claude-cli": ClaudeCLI, "codex-cli": CodexCLI} {
		got, err := ParseProvider(in)
		if err != nil || got != want {
			t.Errorf("ParseProvider(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := ParseProvider("gemini"); err == nil {
		t.Error("expected an error for an unknown provider")
	}
	if !Anthropic.NeedsAPIKey() || ClaudeCLI.NeedsAPIKey() || CodexCLI.NeedsAPIKey() {
		t.Error("only the Anthropic provider needs an API key")
	}
}

func TestClaudeCompleteParsesResult(t *testing.T) {
	var gotArgs []string
	var gotStdin string
	c := &Claude{Binary: "claude", Model: "haiku", run: func(_ context.Context, _ string, args []string, stdin string) ([]byte, error) {
		gotArgs, gotStdin = args, stdin
		return []byte(`{"result":"  Änderungen speichern \n","is_error":false}`), nil
	}}

	got, err := c.Complete(context.Background(), "PROMPT")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Änderungen speichern" {
		t.Errorf("got %q", got)
	}
	if gotStdin != "PROMPT" {
		t.Errorf("prompt must go through stdin, got %q", gotStdin)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"-p", "--tools ", "--no-session-persistence", "--model haiku"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "--bare") {
		t.Error("--bare would skip the subscription login")
	}
}

func TestClaudeCompleteSurfacesErrors(t *testing.T) {
	c := &Claude{run: func(context.Context, string, []string, string) ([]byte, error) {
		return []byte(`{"result":"Not logged in","is_error":true}`), nil
	}}
	if _, err := c.Complete(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "Not logged in") {
		t.Errorf("err = %v", err)
	}
}

func TestCodexArgsOmitModelByDefault(t *testing.T) {
	args := (&Codex{}).args("out.txt")
	if reflect.DeepEqual(args, nil) || strings.Contains(strings.Join(args, " "), "-m ") {
		t.Errorf("args = %v", args)
	}
	if args[len(args)-1] != "-" {
		t.Error("prompt must be read from stdin")
	}
	withModel := strings.Join((&Codex{Model: "gpt-x"}).args("out.txt"), " ")
	if !strings.Contains(withModel, "-m gpt-x") {
		t.Errorf("args = %q", withModel)
	}
}

func TestCodexCompleteReadsOutputFile(t *testing.T) {
	c := &Codex{run: func(_ context.Context, _ string, args []string, _ string) ([]byte, error) {
		for i, a := range args {
			if a == "-o" {
				return nil, os.WriteFile(args[i+1], []byte("Guardar cambios\n"), 0o600)
			}
		}
		t.Error("no -o argument")
		return nil, nil
	}}
	got, err := c.Complete(context.Background(), "x")
	if err != nil || got != "Guardar cambios" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestCodexCompleteRejectsEmptyOutput(t *testing.T) {
	c := &Codex{run: func(context.Context, string, []string, string) ([]byte, error) { return nil, nil }}
	if _, err := c.Complete(context.Background(), "x"); err == nil {
		t.Error("expected an error for an empty response")
	}
}
