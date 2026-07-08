package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/project"
	"github.com/MobilePur/xlocal/internal/ui"
)

var deintegrateCmd = &cobra.Command{
	Use:   "deintegrate",
	Short: "Remove xlocal from this project: delete the config in the current folder",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		cfgPath := filepath.Join(cwd, project.ConfigFileName)
		if _, err := os.Stat(cfgPath); err != nil {
			if upwards, ok := project.FindConfigUpwards(cwd); ok {
				return fmt.Errorf("no %s in this folder — the config lives at %s, run deintegrate there", project.ConfigFileName, upwards)
			}
			return fmt.Errorf("no %s found in %s — nothing to remove", project.ConfigFileName, cwd)
		}

		var confirmed bool
		err = huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Delete %s?", cfgPath)).
				Description("API keys stay in the keychain — manage them with: xlocal keys").
				Value(&confirmed),
		)).Run()
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println(ui.Dim.Render("Nothing deleted."))
			return nil
		}

		if err := os.Remove(cfgPath); err != nil {
			return err
		}
		fmt.Printf("%s Deleted %s\n", ui.Success.Render("✓"), cfgPath)
		fmt.Println(ui.Dim.Render("Set xlocal up again anytime with: xlocal init"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deintegrateCmd)
}
