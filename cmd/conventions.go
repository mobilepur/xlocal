package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/ui"
)

var conventionsCmd = &cobra.Command{
	Use:   "conventions",
	Short: "Show the catalog conventions xlocal relies on",
	Long:  "Explains how xlocal reads your String Catalogs and what it sends to the model, so you know how to write keys, comments and translations that translate well.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.Title.Render("Catalog conventions"))
		fmt.Println("xlocal builds its translation prompts from your catalog. It works best when:")
		fmt.Println()
		fmt.Println(ui.Accent.Render("  1. English is the source language."))
		fmt.Println("     Write source strings in English — all other languages are translated from it.")
		fmt.Println()
		fmt.Println(ui.Accent.Render("  2. Developer comments are written in English."))
		fmt.Println("     The comment of a key is sent to the model. It is your main way to give")
		fmt.Println("     context: what the string is (\"button label\", \"empty-state title\") and")
		fmt.Println("     where it appears.")
		fmt.Println()
		fmt.Println(ui.Accent.Render("  3. Existing translations are the reference."))
		fmt.Println("     Every existing translation of a key is included in the prompt, so new")
		fmt.Println("     languages stay consistent with your established terminology. The better")
		fmt.Println("     your existing translations, the better the new ones.")
		fmt.Println()
		fmt.Println(ui.Accent.Render("  4. Brand and product names go into untranslatableWords."))
		fmt.Println("     xlocal instructs the model to keep them exactly as written and warns")
		fmt.Println("     you when one was translated anyway.")
		fmt.Println()
		fmt.Println(ui.Dim.Render("Formality (Sie/Vous vs. du/tu) is controlled per language via formalLanguages."))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(conventionsCmd)
}
