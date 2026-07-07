package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/ui"
)

var flagJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the translation status of all catalogs (read-only, no API key needed)",
	RunE: func(cmd *cobra.Command, args []string) error {
		pc, err := resolveProject()
		if err != nil {
			return err
		}

		reports, err := analyzeProject(pc)
		if err != nil {
			return err
		}

		if flagJSON {
			return printStatusJSON(reports, pc)
		}

		fmt.Println(ui.Dim.Render("Project: " + pc.Root))
		printStatus(reports, pc.Root)

		total := 0
		for _, r := range reports {
			total += len(r.Missing)
		}
		if total == 0 {
			fmt.Println("\n" + ui.Success.Render("✓ All translations are complete!"))
		} else {
			fmt.Printf("\n%s\n", ui.Warn.Render(fmt.Sprintf("%d translations missing — run xlocal to translate them.", total)))
		}
		return nil
	},
}

type statusJSONFile struct {
	Path         string         `json:"path"`
	TotalStrings int            `json:"totalStrings"`
	Missing      int            `json:"missing"`
	PerLanguage  map[string]int `json:"perLanguage"`
}

func printStatusJSON(reports []*analyze.Report, pc *projectContext) error {
	files := make([]statusJSONFile, 0, len(reports))
	for _, r := range reports {
		perLanguage := map[string]int{}
		for _, m := range r.Missing {
			perLanguage[m.TargetLanguage]++
		}
		files = append(files, statusJSONFile{
			Path:         displayPath(pc.Root, r.FilePath),
			TotalStrings: r.TotalStrings,
			Missing:      len(r.Missing),
			PerLanguage:  perLanguage,
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(map[string]interface{}{
		"root":  pc.Root,
		"files": files,
	})
}

func init() {
	statusCmd.Flags().BoolVar(&flagJSON, "json", false, "machine-readable output")
	rootCmd.AddCommand(statusCmd)
}
