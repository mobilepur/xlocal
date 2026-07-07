package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/keychain"
	"github.com/MobilePur/xlocal/internal/project"
	"github.com/MobilePur/xlocal/internal/settings"
	"github.com/MobilePur/xlocal/internal/ui"
	"github.com/MobilePur/xlocal/internal/xcstrings"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up xlocal for this project: config skeleton and API key check",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		if existing, ok := project.FindConfigUpwards(cwd); ok {
			return fmt.Errorf("there is already a config at %s — edit it with: xlocal config", existing)
		}

		dir, err := resolveInitDir(cwd)
		if err != nil {
			return err
		}

		if _, err := createConfigSkeleton(dir); err != nil {
			return err
		}

		if err := checkAPIKey(); err != nil {
			return err
		}

		fmt.Println()
		fmt.Println(ui.Title.Render("Next steps"))
		fmt.Println("  1. Fill in " + project.ConfigFileName + " — edit it with: " + ui.Accent.Render("xlocal config"))
		fmt.Println("  2. See what's missing: " + ui.Accent.Render("xlocal status"))
		fmt.Println("  3. Translate: " + ui.Accent.Render("xlocal"))
		if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
			fmt.Println(ui.Dim.Render("Tip: mention xlocal in your CLAUDE.md so AI agents know how to translate here."))
		}
		return nil
	},
}

// resolveInitDir checks that init runs in a sensible place and guides the
// user to the project root if not: catalogs below the working directory win,
// then discovered projects below, then a project root further up.
func resolveInitDir(cwd string) (string, error) {
	catalogs, err := project.FindCatalogs(cwd, nil)
	if err != nil {
		return "", err
	}
	if len(catalogs) > 0 {
		return cwd, nil
	}

	candidates, err := project.DiscoverProjects(cwd, 4)
	if err != nil {
		return "", err
	}
	var open []project.Candidate
	for _, c := range candidates {
		if !c.HasConfig {
			open = append(open, c)
		}
	}
	if len(open) > 0 {
		fmt.Println(ui.Dim.Render("No String Catalogs found in this folder, but there are projects below:"))
		options := make([]huh.Option[int], 0, len(open))
		for i, c := range open {
			rel, relErr := filepath.Rel(cwd, c.Dir)
			if relErr != nil || rel == "." {
				rel = c.Dir
			}
			options = append(options, huh.NewOption(rel+"  "+ui.Dim.Render("("+c.Kind+")"), i))
		}
		var picked int
		err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[int]().
				Title("Where should the config go?").
				Options(options...).
				Value(&picked),
		)).Run()
		if err != nil {
			return "", err
		}
		return open[picked].Dir, nil
	}

	if root, ok := findProjectRootUpwards(cwd); ok {
		var moveUp bool
		err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("You seem to be inside %s. Create the config there?", root)).
				Description("The config belongs in the project root so xlocal works from any subfolder.").
				Value(&moveUp),
		)).Run()
		if err != nil {
			return "", err
		}
		if moveUp {
			return root, nil
		}
		return cwd, nil
	}

	return "", fmt.Errorf("this doesn't look like an Xcode project (no String Catalogs, no .xcodeproj) — cd into your project root and run xlocal init again")
}

// findProjectRootUpwards walks from dir towards the filesystem root and
// returns the first directory containing an Xcode project, workspace or
// Swift package manifest.
func findProjectRootUpwards(dir string) (string, bool) {
	for {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				name := e.Name()
				if strings.HasSuffix(name, ".xcodeproj") || strings.HasSuffix(name, ".xcworkspace") || name == "Package.swift" {
					return dir, true
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// configSkeleton mirrors project.Config without omitempty, so the created
// file shows every field there is to fill in.
type configSkeleton struct {
	TargetLanguages     []string `json:"targetLanguages"`
	BaseLanguages       []string `json:"baseLanguages"`
	UntranslatableWords []string `json:"untranslatableWords"`
	FormalLanguages     []string `json:"formalLanguages"`
	Model               string   `json:"model"`
	Exclude             []string `json:"exclude"`
}

// createConfigSkeleton writes an xlocal-config.json in dir with every field
// present and empty, prefilling only the languages detected in existing
// catalogs.
func createConfigSkeleton(dir string) (*project.Config, error) {
	detected, sourceLangs := detectLanguages(dir)

	skeleton := configSkeleton{
		TargetLanguages:     append([]string{}, detected...),
		BaseLanguages:       append([]string{}, sourceLangs...),
		UntranslatableWords: []string{},
		FormalLanguages:     []string{},
		Exclude:             []string{},
	}

	path := filepath.Join(dir, project.ConfigFileName)
	data, err := json.MarshalIndent(skeleton, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return nil, err
	}

	fmt.Printf("%s Created %s\n", ui.Success.Render("✓"), path)
	if len(detected) > 0 {
		fmt.Println(ui.Dim.Render("Prefilled with the languages found in your catalogs: " + strings.Join(detected, ", ")))
	} else {
		fmt.Println(ui.Warn.Render("⚠ No catalogs with languages found — fill in targetLanguages before translating."))
	}
	fmt.Println(ui.Dim.Render("Check it into your repository — it contains no secrets."))

	return &project.Config{
		TargetLanguages: skeleton.TargetLanguages,
		BaseLanguages:   skeleton.BaseLanguages,
	}, nil
}

// checkAPIKey verifies that an Anthropic API key is registered, offering to
// add one right away if not.
func checkAPIKey() error {
	s, err := settings.Load()
	if err != nil {
		return err
	}
	if len(s.Keys) > 0 {
		name := s.DefaultKey
		if name == "" {
			name = s.Keys[0]
		}
		fmt.Printf("%s API key %q is set up.\n", ui.Success.Render("✓"), name)
		return nil
	}

	fmt.Println(ui.Warn.Render("⚠ No Anthropic API key stored yet."))
	var addNow bool
	err = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Add an API key now?").
			Description("Stored in the macOS keychain, never on disk. Get one at console.anthropic.com.").
			Value(&addNow),
	)).Run()
	if err != nil {
		return err
	}
	if !addNow {
		fmt.Println(ui.Dim.Render("Add one later with: xlocal keys add"))
		return nil
	}
	_, err = addKeyInteractive(keychain.New(), s)
	return err
}

// detectLanguages scans the project's catalogs and returns all languages
// seen (sorted) plus the distinct source languages.
func detectLanguages(dir string) (all []string, sourceLangs []string) {
	catalogs, err := project.FindCatalogs(dir, nil)
	if err != nil {
		return nil, nil
	}

	langSet := map[string]bool{}
	sourceSet := map[string]bool{}
	for _, path := range catalogs {
		catalog, err := xcstrings.Load(path)
		if err != nil {
			continue
		}
		if catalog.SourceLanguage != "" {
			langSet[catalog.SourceLanguage] = true
			sourceSet[catalog.SourceLanguage] = true
		}
		for _, entry := range catalog.Strings {
			for lang := range entry.Localizations {
				langSet[lang] = true
			}
		}
	}

	for lang := range langSet {
		all = append(all, lang)
	}
	sort.Strings(all)
	for lang := range sourceSet {
		sourceLangs = append(sourceLangs, lang)
	}
	sort.Strings(sourceLangs)
	return all, sourceLangs
}

func init() {
	rootCmd.AddCommand(initCmd)
}
