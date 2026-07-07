package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/project"
	"github.com/MobilePur/xlocal/internal/settings"
	"github.com/MobilePur/xlocal/internal/ui"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Open the project config in your editor",
	Long: `Opens the project's xlocal-config.json in $VISUAL or $EDITOR (falling back
to vim) and validates it afterwards. Use "xlocal config global" for the
global settings (model, default key).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		cfgPath, ok := project.FindConfigUpwards(cwd)
		if !ok {
			return fmt.Errorf("no %s found from %s upwards — create one with: xlocal init", project.ConfigFileName, cwd)
		}

		editor := editorCommand()
		fmt.Println(ui.Dim.Render("Opening " + cfgPath + " with " + strings.Join(editor, " ")))

		c := exec.Command(editor[0], append(editor[1:], cfgPath)...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("editor %s: %w", editor[0], err)
		}

		if _, err := project.LoadConfig(cfgPath); err != nil {
			return fmt.Errorf("saved, but the config has a problem: %w", err)
		}
		fmt.Printf("%s Config is valid.\n", ui.Success.Render("✓"))
		return nil
	},
}

// editorCommand resolves the editor to use: $VISUAL, then $EDITOR, then vim.
// The variables may contain arguments (e.g. "code --wait").
func editorCommand() []string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return strings.Fields(v)
		}
	}
	return []string{"vim"}
}

var configGlobalCmd = &cobra.Command{
	Use:   "global",
	Short: "Show or change the global settings (model, default key)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := settings.Load()
		if err != nil {
			return err
		}

		fmt.Println(ui.Dim.Render("Settings file: " + settings.Path()))

		model := s.Model
		if model == "" {
			model = "sonnet"
		}

		fields := []huh.Field{
			huh.NewSelect[string]().
				Title("Model").
				Description("Alias resolves to the newest matching model. Projects can override this in xlocal-config.json.").
				Options(
					huh.NewOption("sonnet — balanced (recommended)", "sonnet"),
					huh.NewOption("haiku — fastest, cheapest", "haiku"),
					huh.NewOption("opus — highest quality", "opus"),
				).
				Value(&model),
		}

		defaultKey := s.DefaultKey
		if len(s.Keys) > 1 {
			keyOptions := make([]huh.Option[string], 0, len(s.Keys))
			for _, name := range s.Keys {
				keyOptions = append(keyOptions, huh.NewOption(name, name))
			}
			fields = append(fields, huh.NewSelect[string]().
				Title("Default API key").
				Options(keyOptions...).
				Value(&defaultKey))
		}

		if err := huh.NewForm(huh.NewGroup(fields...)).Run(); err != nil {
			return err
		}

		s.Model = model
		if defaultKey != "" {
			s.DefaultKey = defaultKey
		}
		if err := s.Save(); err != nil {
			return err
		}

		fmt.Printf("%s Saved: model %s", ui.Success.Render("✓"), ui.Accent.Render(model))
		if s.DefaultKey != "" {
			fmt.Printf(", default key %q", s.DefaultKey)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	configCmd.AddCommand(configGlobalCmd)
	rootCmd.AddCommand(configCmd)
}
