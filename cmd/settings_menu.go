package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/keychain"
	"github.com/MobilePur/xlocal/internal/llm"
	"github.com/MobilePur/xlocal/internal/project"
	"github.com/MobilePur/xlocal/internal/settings"
	"github.com/MobilePur/xlocal/internal/ui"
)

// settingsMenu is the interactive editor for the global settings: provider,
// model and API keys. It is used by `xlocal init` for the first setup and by
// `xlocal config global` to change things later. Every change is saved
// immediately, so leaving at any point keeps what was set so far.
type settingsMenu struct {
	s        *settings.Settings
	store    keychain.SecretStore
	lookPath func(string) (string, error)
}

func newSettingsMenu(s *settings.Settings) *settingsMenu {
	return &settingsMenu{s: s, store: keychain.New(), lookPath: exec.LookPath}
}

// runMenuForm runs a single-group form in which Esc backs out like Ctrl+C
// (huh only binds Ctrl+C by default).
func runMenuForm(fields ...huh.Field) error {
	keys := huh.NewDefaultKeyMap()
	keys.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"))
	return huh.NewForm(huh.NewGroup(fields...)).WithKeyMap(keys).Run()
}

// pick shows a select. ok is false when the user backs out with Esc/Ctrl+C.
func pick(title, description string, options []huh.Option[string], value string) (chosen string, ok bool, err error) {
	chosen = value
	err = runMenuForm(huh.NewSelect[string]().Title(title).Description(description).Options(options...).Value(&chosen))
	if errors.Is(err, huh.ErrUserAborted) {
		return "", false, nil
	}
	return chosen, err == nil, err
}

// effectiveProvider is the globally configured provider (Anthropic if unset).
func effectiveProvider(s *settings.Settings) llm.Provider {
	p, err := llm.ParseProvider(s.Provider)
	if err != nil {
		return llm.Anthropic
	}
	return p
}

// providerName is the short display name of a provider.
func providerName(p llm.Provider) string {
	switch p {
	case llm.ClaudeCLI:
		return "Claude subscription (claude-cli)"
	case llm.CodexCLI:
		return "ChatGPT subscription (codex-cli)"
	}
	return "Anthropic API key (anthropic)"
}

// cliBinary is the executable a CLI provider needs ("" for the API).
func cliBinary(p llm.Provider) string {
	switch p {
	case llm.ClaudeCLI:
		return "claude"
	case llm.CodexCLI:
		return "codex"
	}
	return ""
}

// modelFits reports whether a model value makes sense for a provider. An
// empty model always fits (the provider default). Codex has no Anthropic
// aliases, the other providers need Anthropic aliases or IDs.
func modelFits(p llm.Provider, model string) bool {
	if model == "" {
		return true
	}
	anthropicLike := model == "sonnet" || model == "haiku" || model == "opus" || strings.HasPrefix(model, "claude-")
	if p == llm.CodexCLI {
		return !anthropicLike
	}
	return anthropicLike
}

// modelLabel describes the model the provider will use.
func modelLabel(p llm.Provider, model string) string {
	if p == llm.CodexCLI {
		if model == "" || !modelFits(p, model) {
			return "Codex default"
		}
		return model
	}
	if model == "" {
		return "sonnet"
	}
	return model
}

// modelHelp is the description of the model menu; only the Anthropic-based
// providers know aliases.
func modelHelp(p llm.Provider) string {
	if p == llm.CodexCLI {
		return "Codex picks its own default model. Choose another name only if your ChatGPT account supports it. A project's xlocal-config.json can override this."
	}
	return "Aliases resolve to the newest matching model. A project's xlocal-config.json can override this."
}

// keysLabel describes the API keys in the main menu. With a subscription
// provider a missing key is nothing to fix.
func (m *settingsMenu) keysLabel(p llm.Provider) string {
	if p.NeedsAPIKey() {
		return m.providerStatus(llm.Anthropic)
	}
	if len(m.s.Keys) == 0 {
		return "not needed with a subscription"
	}
	return m.providerStatus(llm.Anthropic) + ", not used with a subscription"
}

// providerStatus is the one-line hint shown next to a provider in the menu.
func (m *settingsMenu) providerStatus(p llm.Provider) string {
	if bin := cliBinary(p); bin != "" {
		if _, err := m.lookPath(bin); err != nil {
			return bin + " not found"
		}
		return bin + " found"
	}
	switch n := len(m.s.Keys); n {
	case 0:
		return "no API key yet"
	case 1:
		return "1 key stored"
	default:
		return fmt.Sprintf("%d keys stored", n)
	}
}

// Run shows the top-level menu until the user is done.
func (m *settingsMenu) Run() error {
	for {
		p := effectiveProvider(m.s)
		options := []huh.Option[string]{
			huh.NewOption(fmt.Sprintf("Provider — %s  ›", providerName(p)), "provider"),
			huh.NewOption(fmt.Sprintf("Model — %s  ›", modelLabel(p, m.s.Model)), "model"),
			huh.NewOption(fmt.Sprintf("API keys — %s  ›", m.keysLabel(p)), "keys"),
			huh.NewOption("Done", "done"),
		}
		choice, ok, err := pick("xlocal settings", "Global defaults. A project's xlocal-config.json can override provider and model.", options, "provider")
		if err != nil || !ok {
			return err
		}
		switch choice {
		case "provider":
			err = m.providerMenu()
		case "model":
			err = m.modelMenu()
		case "keys":
			err = m.keysMenu()
		default:
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// providerMenu lets the user pick a provider and then walks through what that
// provider needs (an API key or an installed CLI).
func (m *settingsMenu) providerMenu() error {
	options := make([]huh.Option[string], 0, len(llm.Providers))
	for _, p := range llm.Providers {
		options = append(options, huh.NewOption(fmt.Sprintf("%s — %s", providerName(p), m.providerStatus(p)), string(p)))
	}
	chosen, ok, err := pick("Provider", "How should xlocal reach a model? The subscription providers run the CLI you are already signed in to.", options, string(effectiveProvider(m.s)))
	if err != nil || !ok {
		return err
	}

	p := llm.Provider(chosen)
	m.s.Provider = chosen
	if !modelFits(p, m.s.Model) {
		m.s.Model = ""
		fmt.Println(ui.Dim.Render("Model reset to the default for " + providerName(p) + "."))
	}
	if err := m.s.Save(); err != nil {
		return err
	}
	fmt.Printf("%s Provider: %s\n", ui.Success.Render("✓"), ui.Accent.Render(providerName(p)))

	return m.afterProvider(p)
}

// afterProvider is the provider-specific submenu: checking the CLI or setting
// up an API key.
func (m *settingsMenu) afterProvider(p llm.Provider) error {
	if bin := cliBinary(p); bin != "" {
		if path, err := m.lookPath(bin); err != nil {
			hint := "install Claude Code and sign in with `claude`"
			if p == llm.CodexCLI {
				hint = "install Codex and sign in with `codex login`"
			}
			fmt.Println(ui.Warn.Render(fmt.Sprintf("⚠ %s not found in PATH — %s.", bin, hint)))
		} else {
			fmt.Println(ui.Dim.Render(fmt.Sprintf("Using %s. Make sure you are signed in with your subscription.", path)))
		}
		fmt.Println(ui.Dim.Render("Each string starts a CLI process (slower than the API) and counts against your subscription limits."))
		return nil
	}

	if len(m.s.Keys) == 0 {
		var addNow bool
		err := runMenuForm(huh.NewConfirm().
			Title("No API key stored yet. Add one now?").
			Description("Stored in the macOS keychain, never on disk. Get one at console.anthropic.com.").
			Value(&addNow))
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		if err != nil {
			return err
		}
		if addNow {
			_, err = addKeyInteractive(m.store, m.s)
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}
		fmt.Println(ui.Dim.Render("Add one later with: xlocal config global"))
	}
	return nil
}

// modelMenu picks the model for the current provider.
func (m *settingsMenu) modelMenu() error {
	p := effectiveProvider(m.s)

	var options []huh.Option[string]
	if p == llm.CodexCLI {
		options = []huh.Option[string]{
			huh.NewOption("Codex default (recommended)", ""),
			huh.NewOption("Other model name…", "custom"),
		}
	} else {
		options = []huh.Option[string]{
			huh.NewOption("sonnet — balanced (recommended)", "sonnet"),
			huh.NewOption("haiku — fastest, cheapest", "haiku"),
			huh.NewOption("opus — highest quality", "opus"),
			huh.NewOption("Other model ID…", "custom"),
		}
	}

	current := m.s.Model
	if current == "" && p != llm.CodexCLI {
		current = "sonnet"
	}
	chosen, ok, err := pick("Model", modelHelp(p), options, current)
	if err != nil || !ok {
		return err
	}

	if chosen == "custom" {
		var name string
		err := runMenuForm(
			huh.NewInput().Title("Model name").Value(&name).Validate(func(v string) error {
				v = strings.TrimSpace(v)
				if v == "" {
					return fmt.Errorf("name must not be empty")
				}
				if !modelFits(p, v) {
					if p == llm.CodexCLI {
						return fmt.Errorf("Codex has no Anthropic aliases — enter a Codex model name")
					}
					return fmt.Errorf("enter sonnet, haiku, opus or a full claude-… model ID")
				}
				return nil
			}),
		)
		if errors.Is(err, huh.ErrUserAborted) {
			return nil
		}
		if err != nil {
			return err
		}
		chosen = strings.TrimSpace(name)
	}

	m.s.Model = chosen
	if err := m.s.Save(); err != nil {
		return err
	}
	fmt.Printf("%s Model: %s\n", ui.Success.Render("✓"), ui.Accent.Render(modelLabel(p, chosen)))
	return nil
}

// keysMenu manages the stored API keys.
func (m *settingsMenu) keysMenu() error {
	for {
		var description string
		if len(m.s.Keys) == 0 {
			description = "No keys stored yet."
		} else {
			lines := make([]string, 0, len(m.s.Keys))
			for _, name := range m.s.Keys {
				marker := "○"
				if name == m.s.DefaultKey {
					marker = "● (default)"
				}
				lines = append(lines, fmt.Sprintf("%s %s", marker, name))
			}
			description = strings.Join(lines, "\n")
		}

		options := []huh.Option[string]{huh.NewOption("Add a key", "add")}
		if len(m.s.Keys) > 1 {
			options = append(options, huh.NewOption("Choose the default key", "default"))
		}
		if len(m.s.Keys) > 0 {
			options = append(options, huh.NewOption("Remove a key", "remove"))
		}
		options = append(options, huh.NewOption("Back", "back"))

		choice, ok, err := pick("API keys", description, options, "add")
		if err != nil || !ok || choice == "back" {
			return err
		}

		switch choice {
		case "add":
			_, err = addKeyInteractive(m.store, m.s)
		case "default":
			err = m.chooseKey("Default API key", func(name string) error {
				m.s.DefaultKey = name
				return m.s.Save()
			})
		case "remove":
			err = m.chooseKey("Remove which key?", func(name string) error {
				if err := m.store.Delete(name); err != nil {
					return err
				}
				m.s.RemoveKey(name)
				if err := m.s.Save(); err != nil {
					return err
				}
				fmt.Printf("%s Key %q removed.\n", ui.Success.Render("✓"), name)
				return nil
			})
		}
		if errors.Is(err, huh.ErrUserAborted) {
			continue
		}
		if err != nil {
			return err
		}
	}
}

func (m *settingsMenu) chooseKey(title string, apply func(name string) error) error {
	options := make([]huh.Option[string], 0, len(m.s.Keys))
	for _, name := range m.s.Keys {
		options = append(options, huh.NewOption(name, name))
	}
	name, ok, err := pick(title, "", options, m.s.DefaultKey)
	if err != nil || !ok {
		return err
	}
	return apply(name)
}

// setupProviderForInit runs during `xlocal init` and always asks which
// provider to use, preselecting the current one. projectConfig is the config
// in the initialized folder; if it names its own provider, that one wins and
// the user is told so.
func setupProviderForInit(projectConfig string) error {
	s, err := settings.Load()
	if err != nil {
		return err
	}
	m := newSettingsMenu(s)

	fmt.Println(ui.Title.Render("Provider") + " — which provider should xlocal use?")
	if err := m.providerMenu(); err != nil {
		if !errors.Is(err, huh.ErrUserAborted) {
			return err
		}
	}
	if s.Provider == "" {
		fmt.Println(ui.Dim.Render("No provider chosen yet — set one with: xlocal config global"))
	} else {
		fmt.Println(ui.Dim.Render("Saved as your global default. Change it any time with: xlocal config global"))
	}

	if cfg, err := project.LoadConfigRaw(projectConfig); err == nil && cfg.Provider != "" {
		fmt.Println(ui.Warn.Render(fmt.Sprintf("⚠ This project's config sets provider %q, which overrides your global default here.", cfg.Provider)))
	}
	return nil
}

// needsProviderSetup reports whether nothing usable is set up yet, so xlocal
// has to ask for a provider: no subscription provider and no API key
// globally, no --key, and at least one of the catalogs to translate has no
// provider in its project config. Choosing the API without adding a key does
// not count as set up.
func needsProviderSetup(s *settings.Settings, keyFlag string, batch []analyze.Missing, pc *projectContext) bool {
	if !effectiveProvider(s).NeedsAPIKey() || len(s.Keys) > 0 || keyFlag != "" {
		return false
	}
	for _, item := range batch {
		if pc.configFor(item.FilePath).Provider == "" {
			return true
		}
	}
	return false
}

// setupProviderForFirstRun asks for a provider before the first translation.
// It fails if the user leaves without a usable choice.
func setupProviderForFirstRun(s *settings.Settings) error {
	fmt.Println(ui.Title.Render("Welcome to xlocal!") + " Which provider should xlocal use?")
	if err := newSettingsMenu(s).providerMenu(); err != nil {
		return err
	}
	if s.Provider == "" {
		return huh.ErrUserAborted
	}
	if effectiveProvider(s).NeedsAPIKey() && len(s.Keys) == 0 {
		return fmt.Errorf("no API key stored yet — add one or pick a subscription provider with: xlocal config global")
	}
	return nil
}
