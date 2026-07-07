package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/project"
	"github.com/MobilePur/xlocal/internal/ui"
	"github.com/MobilePur/xlocal/internal/xcstrings"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create an xlocal-config.json for this project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		if existing, ok := project.FindConfigUpwards(cwd); ok {
			return fmt.Errorf("there is already a config at %s", existing)
		}

		_, err = createConfigInteractive(cwd)
		return err
	},
}

// createConfigInteractive builds an xlocal-config.json in dir, proposing
// languages detected from the project's existing catalogs.
func createConfigInteractive(dir string) (*project.Config, error) {
	detected, sourceLangs := detectLanguages(dir)

	fmt.Println(ui.Title.Render("Setting up " + project.ConfigFileName))
	if len(detected) > 0 {
		fmt.Println(ui.Dim.Render("Languages found in your catalogs: " + strings.Join(detected, ", ")))
	}

	var targets []string
	var targetsInput string
	var form *huh.Form

	if len(detected) > 0 {
		options := make([]huh.Option[string], 0, len(detected))
		for _, lang := range detected {
			options = append(options, huh.NewOption(ui.Lang(lang)+"  "+lang, lang).Selected(true))
		}
		form = huh.NewForm(huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Target languages").
				Description("Languages xlocal should keep complete. Edit the config file later to add more.").
				Options(options...).
				Value(&targets).
				Validate(func(v []string) error {
					if len(v) == 0 {
						return fmt.Errorf("select at least one language")
					}
					return nil
				}),
		))
	} else {
		form = huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Target languages (comma-separated codes)").
				Description("e.g. en, de, fr — no catalogs with languages were found to propose.").
				Value(&targetsInput).
				Validate(func(v string) error {
					if len(parseLangList(v)) == 0 {
						return fmt.Errorf("enter at least one language code")
					}
					return nil
				}),
		))
	}
	if err := form.Run(); err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		targets = parseLangList(targetsInput)
	}

	// Base languages: the catalogs' source languages that are also targets.
	var baseLanguages []string
	for _, lang := range sourceLangs {
		for _, t := range targets {
			if t == lang {
				baseLanguages = append(baseLanguages, lang)
				break
			}
		}
	}

	var untranslatableInput string
	var formalLanguages []string

	formalOptions := make([]huh.Option[string], 0, len(targets))
	for _, lang := range targets {
		formalOptions = append(formalOptions, huh.NewOption(ui.Lang(lang)+"  "+lang, lang))
	}

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Untranslatable words (optional, comma-separated)").
			Description("Brand and product names that must stay exactly as written, e.g. your app's name.").
			Value(&untranslatableInput),
		huh.NewMultiSelect[string]().
			Title("Formal address (optional)").
			Description("Languages that should address the user formally (Sie/Vous instead of du/tu).").
			Options(formalOptions...).
			Value(&formalLanguages),
	)).Run()
	if err != nil {
		return nil, err
	}

	cfg := &project.Config{
		TargetLanguages:     targets,
		BaseLanguages:       baseLanguages,
		UntranslatableWords: parseWordList(untranslatableInput),
		FormalLanguages:     formalLanguages,
	}

	path := filepath.Join(dir, project.ConfigFileName)
	if err := cfg.Save(path); err != nil {
		return nil, err
	}

	fmt.Printf("%s Created %s\n", ui.Success.Render("✓"), path)
	fmt.Println(ui.Dim.Render("Check it into your repository — it contains no secrets. Run xlocal to start translating."))
	return cfg, nil
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

func parseLangList(s string) []string {
	return splitTrimmed(s)
}

func parseWordList(s string) []string {
	return splitTrimmed(s)
}

func splitTrimmed(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func init() {
	rootCmd.AddCommand(initCmd)
}
