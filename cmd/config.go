package cmd

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/settings"
	"github.com/MobilePur/xlocal/internal/ui"
)

var configCmd = &cobra.Command{
	Use:   "config",
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
	rootCmd.AddCommand(configCmd)
}
