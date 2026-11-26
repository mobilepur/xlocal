package cmd

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/llm"
	"github.com/MobilePur/xlocal/internal/project"
	"github.com/MobilePur/xlocal/internal/settings"
)

func TestModelFits(t *testing.T) {
	cases := []struct {
		p     llm.Provider
		model string
		want  bool
	}{
		{llm.Anthropic, "", true},
		{llm.Anthropic, "sonnet", true},
		{llm.ClaudeCLI, "claude-opus-4-8", true},
		{llm.ClaudeCLI, "gpt-x", false},
		{llm.CodexCLI, "", true},
		{llm.CodexCLI, "gpt-x", true},
		{llm.CodexCLI, "haiku", false},
		{llm.CodexCLI, "claude-sonnet-5", false},
	}
	for _, c := range cases {
		if got := modelFits(c.p, c.model); got != c.want {
			t.Errorf("modelFits(%s, %q) = %v, want %v", c.p, c.model, got, c.want)
		}
	}
}

func TestModelLabel(t *testing.T) {
	if got := modelLabel(llm.Anthropic, ""); got != "sonnet" {
		t.Errorf("anthropic default = %q", got)
	}
	if got := modelLabel(llm.CodexCLI, "sonnet"); got != "Codex default" {
		t.Errorf("codex with anthropic alias = %q", got)
	}
	if got := modelLabel(llm.CodexCLI, "gpt-x"); got != "gpt-x" {
		t.Errorf("codex model = %q", got)
	}
}

func TestEffectiveProviderFallsBackToAnthropic(t *testing.T) {
	for _, in := range []string{"", "nonsense"} {
		if got := effectiveProvider(&settings.Settings{Provider: in}); got != llm.Anthropic {
			t.Errorf("effectiveProvider(%q) = %s", in, got)
		}
	}
	if got := effectiveProvider(&settings.Settings{Provider: "codex-cli"}); got != llm.CodexCLI {
		t.Errorf("got %s", got)
	}
}

func TestProviderStatus(t *testing.T) {
	m := &settingsMenu{s: &settings.Settings{}, lookPath: func(name string) (string, error) {
		if name == "claude" {
			return "/bin/claude", nil
		}
		return "", errors.New("not found")
	}}
	if got := m.providerStatus(llm.ClaudeCLI); got != "claude found" {
		t.Errorf("claude = %q", got)
	}
	if got := m.providerStatus(llm.CodexCLI); got != "codex not found" {
		t.Errorf("codex = %q", got)
	}
	if got := m.providerStatus(llm.Anthropic); !strings.Contains(got, "no API key") {
		t.Errorf("anthropic without key = %q", got)
	}
	m.s.Keys = []string{"a", "b"}
	if got := m.providerStatus(llm.Anthropic); got != "2 keys stored" {
		t.Errorf("anthropic with keys = %q", got)
	}
}

func TestNeedsProviderSetup(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, `{"targetLanguages":["de"]}`)
	pinned := filepath.Join(root, "Pinned")
	writeTestConfig(t, pinned, `{"strategy":"override","targetLanguages":["de"],"provider":"claude-cli"}`)
	resolver, err := project.NewConfigResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	pc := &projectContext{Root: root, Resolver: resolver}
	open := []analyze.Missing{{FilePath: filepath.Join(root, "A.xcstrings")}}
	covered := []analyze.Missing{{FilePath: filepath.Join(pinned, "B.xcstrings")}}

	if !needsProviderSetup(&settings.Settings{}, "", open, pc) {
		t.Error("a fresh install must ask for a provider")
	}
	if needsProviderSetup(&settings.Settings{}, "", covered, pc) {
		t.Error("the project config already names a provider")
	}
	if !needsProviderSetup(&settings.Settings{Provider: "anthropic"}, "", open, pc) {
		t.Error("choosing the API without a key must ask again")
	}
	if needsProviderSetup(&settings.Settings{Provider: "codex-cli"}, "", open, pc) {
		t.Error("a global provider is already set")
	}
	if needsProviderSetup(&settings.Settings{Keys: []string{"default"}}, "", open, pc) {
		t.Error("existing API key users must not be asked")
	}
	if needsProviderSetup(&settings.Settings{}, "work", open, pc) {
		t.Error("--key implies the API provider")
	}
}

func TestModelHelpMentionsAliasesOnlyWhereTheyExist(t *testing.T) {
	if strings.Contains(strings.ToLower(modelHelp(llm.CodexCLI)), "alias") {
		t.Error("Codex has no aliases")
	}
	for _, p := range []llm.Provider{llm.Anthropic, llm.ClaudeCLI} {
		if !strings.Contains(modelHelp(p), "Aliases") {
			t.Errorf("%s help should explain aliases", p)
		}
	}
}

func TestKeysLabel(t *testing.T) {
	m := &settingsMenu{s: &settings.Settings{}}
	if got := m.keysLabel(llm.Anthropic); got != "no API key yet" {
		t.Errorf("anthropic = %q", got)
	}
	if got := m.keysLabel(llm.ClaudeCLI); got != "not needed with a subscription" {
		t.Errorf("subscription = %q", got)
	}
	m.s.Keys = []string{"work"}
	if got := m.keysLabel(llm.CodexCLI); got != "1 key stored, not used with a subscription" {
		t.Errorf("subscription with key = %q", got)
	}
}
