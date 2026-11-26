package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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
global settings (provider, model, default key).`,
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

		// A nested merge config may be partial; its missing fields are supplied
		// by configs above it. Runtime validation checks the effective config for
		// each catalog.
		if _, err := project.LoadConfigRaw(cfgPath); err != nil {
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
	Short: "Change the global settings (provider, model, API keys)",
	Long: `Opens a menu for the global settings: the provider (Anthropic API key,
Claude subscription or ChatGPT subscription), the model and the stored API
keys. Changes are saved immediately. A project's xlocal-config.json can
override provider and model.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := settings.Load()
		if err != nil {
			return err
		}

		fmt.Println(ui.Dim.Render("Settings file: " + settings.Path()))
		return newSettingsMenu(s).Run()
	},
}

func init() {
	configCmd.AddCommand(configGlobalCmd)
	rootCmd.AddCommand(configCmd)
}
