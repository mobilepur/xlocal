package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/huh"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/anthropic"
	"github.com/MobilePur/xlocal/internal/keychain"
	"github.com/MobilePur/xlocal/internal/project"
	"github.com/MobilePur/xlocal/internal/settings"
	"github.com/MobilePur/xlocal/internal/ui"
	"github.com/MobilePur/xlocal/internal/xcstrings"
)

// projectContext is the resolved project a command operates on.
type projectContext struct {
	Root     string
	Resolver *project.ConfigResolver
}

// newProjectContext builds the context for a project rooted at root, preparing
// the resolver that merges nested configs on top of the root config.
func newProjectContext(root string) (*projectContext, error) {
	resolver, err := project.NewConfigResolver(root)
	if err != nil {
		return nil, err
	}
	return &projectContext{Root: root, Resolver: resolver}, nil
}

// configFor returns the effective config for a catalog: the root config with
// every nested config down to the catalog's directory merged on top.
func (pc *projectContext) configFor(catalogPath string) *project.Config {
	return pc.Resolver.Resolve(filepath.Dir(catalogPath))
}

// resolveProject finds the project to work on: first by searching for an
// xlocal config upwards from the working directory, otherwise by scanning
// downwards for Xcode projects and asking the user to pick one.
func resolveProject() (*projectContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Anchor at the topmost config in the active merge chain. An override config
	// deliberately stops the upward search and becomes the subtree's root.
	if cfgPath, ok := project.FindTopmostConfig(cwd); ok {
		return newProjectContext(filepath.Dir(cfgPath))
	}

	candidates, err := project.DiscoverProjects(cwd, 4)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no %s and no Xcode project found under %s — run xlocal inside your project, or create a config with: xlocal init", project.ConfigFileName, cwd)
	}

	fmt.Println(ui.Dim.Render(fmt.Sprintf("No %s found here — discovered projects below %s:", project.ConfigFileName, cwd)))

	options := make([]huh.Option[int], 0, len(candidates))
	for i, c := range candidates {
		rel, relErr := filepath.Rel(cwd, c.Dir)
		if relErr != nil || rel == "." {
			rel = c.Dir
		}
		label := rel
		if c.HasConfig {
			label += "  " + ui.Success.Render("(configured)")
		} else {
			label += "  " + ui.Dim.Render("("+c.Kind+", no config yet)")
		}
		options = append(options, huh.NewOption(label, i))
	}

	var picked int
	err = huh.NewForm(huh.NewGroup(
		huh.NewSelect[int]().
			Title("Which project do you want to work on?").
			Options(options...).
			Value(&picked),
	)).Run()
	if err != nil {
		return nil, err
	}

	chosen := candidates[picked]
	if chosen.HasConfig {
		return newProjectContext(chosen.Dir)
	}

	var createNow bool
	err = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("This project has no %s yet. Create one now?", project.ConfigFileName)).
			Value(&createNow),
	)).Run()
	if err != nil {
		return nil, err
	}
	if !createNow {
		return nil, fmt.Errorf("aborted — a config is required (xlocal init)")
	}

	cfg, err := createConfigSkeleton(chosen.Dir)
	if err != nil {
		return nil, err
	}
	if len(cfg.TargetLanguages) == 0 {
		return nil, fmt.Errorf("fill in targetLanguages in %s (xlocal config), then run xlocal again",
			filepath.Join(chosen.Dir, project.ConfigFileName))
	}
	return newProjectContext(chosen.Dir)
}

// resolveAPIKey returns the API key secret to use: the --key override, the
// default key, or — on first run — a freshly added one.
func resolveAPIKey(store keychain.SecretStore, s *settings.Settings, keyFlag string) (string, error) {
	name := keyFlag
	if name == "" {
		name = s.DefaultKey
	}

	if name == "" {
		fmt.Println(ui.Title.Render("Welcome to xlocal!") + " No API key is set up yet.")
		var err error
		name, err = addKeyInteractive(store, s)
		if err != nil {
			return "", err
		}
	}

	if !s.HasKey(name) {
		return "", fmt.Errorf("key %q is not registered (known keys: %s) — add it with: xlocal keys add %s",
			name, strings.Join(s.Keys, ", "), name)
	}

	return store.Get(name)
}

// addKeyInteractive prompts for a key name and secret, stores the secret in
// the keychain, and registers the name in the settings.
func addKeyInteractive(store keychain.SecretStore, s *settings.Settings) (string, error) {
	name := "default"
	var secret string

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Name for this API key").
			Description("You can store multiple keys (e.g. private, work) and switch with --key.").
			Value(&name).
			Validate(func(v string) error {
				if strings.TrimSpace(v) == "" {
					return fmt.Errorf("name must not be empty")
				}
				if s.HasKey(strings.TrimSpace(v)) {
					return fmt.Errorf("key %q already exists", strings.TrimSpace(v))
				}
				return nil
			}),
		huh.NewInput().
			Title("Anthropic API key").
			Description("Stored in the macOS keychain, never on disk. Get one at console.anthropic.com.").
			EchoMode(huh.EchoModePassword).
			Value(&secret).
			Validate(func(v string) error {
				if !strings.HasPrefix(strings.TrimSpace(v), "sk-ant-") {
					return fmt.Errorf("that doesn't look like an Anthropic API key (sk-ant-…)")
				}
				return nil
			}),
	)).Run()
	if err != nil {
		return "", err
	}

	name = strings.TrimSpace(name)
	if err := store.Set(name, strings.TrimSpace(secret)); err != nil {
		return "", err
	}
	s.AddKey(name)
	if err := s.Save(); err != nil {
		return "", err
	}

	fmt.Printf("%s Key %q stored in keychain.\n", ui.Success.Render("✓"), name)
	return name, nil
}

// resolveModel picks the model (project config > global setting > sonnet)
// and resolves aliases to concrete model IDs, cached for a day.
func resolveModel(ctx context.Context, apiKey string, cfg *project.Config, s *settings.Settings) (string, error) {
	alias := configuredModel(cfg, s)

	if strings.HasPrefix(alias, "claude-") {
		return alias, nil
	}
	if id, ok := s.CachedModel(alias, time.Now()); ok {
		return id, nil
	}

	client := anthropic.New(apiKey, "")
	id, err := client.ResolveModel(ctx, alias)
	if err != nil {
		return "", err
	}

	s.SetCachedModel(alias, id, time.Now())
	if err := s.Save(); err != nil {
		return "", err
	}
	return id, nil
}

func configuredModel(cfg *project.Config, s *settings.Settings) string {
	if cfg.Model != "" {
		return cfg.Model
	}
	if s.Model != "" {
		return s.Model
	}
	return "sonnet"
}

// resolveClients prepares the model client effective for every selected
// catalog. Merge configs inherit the root model; an override config can start
// a subtree with its own model.
func resolveClients(ctx context.Context, apiKey string, batch []analyze.Missing, pc *projectContext, s *settings.Settings) (map[string]*anthropic.Client, []string, error) {
	byPath := make(map[string]*anthropic.Client)
	byAlias := make(map[string]*anthropic.Client)
	seenModel := make(map[string]bool)
	var modelIDs []string

	for _, item := range batch {
		if _, ok := byPath[item.FilePath]; ok {
			continue
		}

		cfg := pc.configFor(item.FilePath)
		alias := configuredModel(cfg, s)
		client, ok := byAlias[alias]
		if !ok {
			modelID, err := resolveModel(ctx, apiKey, cfg, s)
			if err != nil {
				return nil, nil, err
			}
			client = anthropic.New(apiKey, modelID)
			byAlias[alias] = client
			if !seenModel[modelID] {
				seenModel[modelID] = true
				modelIDs = append(modelIDs, modelID)
			}
		}
		byPath[item.FilePath] = client
	}

	return byPath, modelIDs, nil
}

// analyzeProject loads and analyzes all catalogs of the project, sorted by
// missing translations (most first), then by path.
func analyzeProject(pc *projectContext) ([]*analyze.Report, error) {
	catalogs, err := pc.Resolver.FindCatalogs()
	if err != nil {
		return nil, err
	}
	if len(catalogs) == 0 {
		return nil, fmt.Errorf("no .xcstrings files found under %s", pc.Root)
	}

	var reports []*analyze.Report
	for _, path := range catalogs {
		cfg := pc.configFor(path)
		if len(cfg.TargetLanguages) == 0 {
			return nil, fmt.Errorf("no targetLanguages in effect for %s — set it in %s",
				displayPath(pc.Root, path), filepath.Join(pc.Root, project.ConfigFileName))
		}
		catalog, err := xcstrings.Load(path)
		if err != nil {
			fmt.Println(ui.Warn.Render(fmt.Sprintf("⚠ skipping %s: %v", displayPath(pc.Root, path), err)))
			continue
		}
		reports = append(reports, analyze.File(path, catalog, cfg.TargetLanguages, cfg.ExcludeKeys))
	}

	sort.SliceStable(reports, func(i, j int) bool {
		if len(reports[i].Missing) != len(reports[j].Missing) {
			return len(reports[i].Missing) > len(reports[j].Missing)
		}
		return reports[i].FilePath < reports[j].FilePath
	})
	return reports, nil
}

// displayPath renders a catalog path relative to the project root.
func displayPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return path
}

// printStatus renders the per-file translation status overview.
func printStatus(reports []*analyze.Report, root string) {
	for _, report := range reports {
		fmt.Printf("\n%s %s\n", "📄", ui.Title.Render(displayPath(root, report.FilePath)))

		missingByLang := map[string]int{}
		for _, m := range report.Missing {
			missingByLang[m.TargetLanguage]++
		}

		langWidth := ui.MaxLangWidth(report.TargetLanguages)
		countWidth := 1
		for _, n := range missingByLang {
			if w := len(fmt.Sprint(n)); w > countWidth {
				countWidth = w
			}
		}

		for _, lang := range report.TargetLanguages {
			if n := missingByLang[lang]; n > 0 {
				fmt.Printf("   %s %s\n", ui.LangPadded(lang, langWidth), ui.Error.Render(fmt.Sprintf("%*d missing", countWidth, n)))
			} else {
				fmt.Printf("   %s %s\n", ui.LangPadded(lang, langWidth), ui.Success.Render(fmt.Sprintf("%*s", countWidth, "✓")))
			}
		}
		fmt.Println(ui.Dim.Render(fmt.Sprintf("   %d strings, %d translations missing", report.TotalStrings, len(report.Missing))))
	}
}

// uniqueKeys returns the distinct keys in order of first appearance.
func uniqueKeys(items []analyze.Missing) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range items {
		if !seen[m.Key] {
			seen[m.Key] = true
			keys = append(keys, m.Key)
		}
	}
	return keys
}
