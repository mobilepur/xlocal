package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags by GoReleaser.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "xlocal",
	Short:   "Translate missing strings in Xcode .xcstrings files using the Anthropic API",
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("not implemented yet")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
