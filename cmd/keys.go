package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/keychain"
	"github.com/MobilePur/xlocal/internal/settings"
	"github.com/MobilePur/xlocal/internal/ui"
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage Anthropic API keys (stored in the macOS keychain)",
}

var keysAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new API key",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := settings.Load()
		if err != nil {
			return err
		}
		_, err = addKeyInteractive(keychain.New(), s)
		return err
	},
}

var keysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the stored API keys",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := settings.Load()
		if err != nil {
			return err
		}
		if len(s.Keys) == 0 {
			fmt.Println("No keys stored yet — add one with: xlocal keys add")
			return nil
		}
		for _, name := range s.Keys {
			if name == s.DefaultKey {
				fmt.Printf("%s %s %s\n", ui.Success.Render("●"), name, ui.Dim.Render("(default)"))
			} else {
				fmt.Printf("%s %s\n", ui.Dim.Render("○"), name)
			}
		}
		return nil
	},
}

var keysRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an API key from the keychain",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		s, err := settings.Load()
		if err != nil {
			return err
		}
		if !s.HasKey(name) {
			return fmt.Errorf("key %q is not registered", name)
		}

		if err := keychain.New().Delete(name); err != nil {
			return err
		}
		s.RemoveKey(name)
		if err := s.Save(); err != nil {
			return err
		}

		fmt.Printf("%s Key %q removed.\n", ui.Success.Render("✓"), name)
		if s.DefaultKey != "" {
			fmt.Println(ui.Dim.Render("Default key is now: " + s.DefaultKey))
		}
		return nil
	},
}

var keysDefaultCmd = &cobra.Command{
	Use:   "default <name>",
	Short: "Set the default API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		s, err := settings.Load()
		if err != nil {
			return err
		}
		if !s.HasKey(name) {
			return fmt.Errorf("key %q is not registered — add it with: xlocal keys add", name)
		}

		s.DefaultKey = name
		if err := s.Save(); err != nil {
			return err
		}
		fmt.Printf("%s Default key is now %q.\n", ui.Success.Render("✓"), name)
		return nil
	},
}

func init() {
	keysCmd.AddCommand(keysAddCmd, keysListCmd, keysRemoveCmd, keysDefaultCmd)
	rootCmd.AddCommand(keysCmd)
}
