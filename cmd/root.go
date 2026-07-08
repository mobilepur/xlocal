package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/anthropic"
	"github.com/MobilePur/xlocal/internal/keychain"
	"github.com/MobilePur/xlocal/internal/settings"
	"github.com/MobilePur/xlocal/internal/translate"
	"github.com/MobilePur/xlocal/internal/ui"
	"github.com/MobilePur/xlocal/internal/xcstrings"
)

// Version is set at build time via -ldflags by GoReleaser.
var Version = "dev"

var (
	flagKey    string
	flagDryRun bool
)

var rootCmd = &cobra.Command{
	Use:   "xlocal",
	Short: "Translate missing strings in Xcode String Catalogs using the Anthropic API",
	Long: `xlocal scans your Xcode project for .xcstrings String Catalogs, shows which
translations are missing, and fills the gaps using the Anthropic API.

Run it from anywhere inside your project (it finds the xlocal-config.json
upwards), or from a parent folder (it discovers projects and asks).`,
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTranslateFlow()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if err == huh.ErrUserAborted {
			fmt.Println(ui.Dim.Render("Cancelled."))
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, ui.Error.Render("✗ ")+err.Error())
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagKey, "key", "", "name of the stored API key to use for this run")
	rootCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show what would be translated without calling the API")
}

func runTranslateFlow() error {
	pc, err := resolveProject()
	if err != nil {
		return err
	}
	fmt.Println(ui.Dim.Render("Project: " + pc.Root))

	reports, err := analyzeProject(pc)
	if err != nil {
		return err
	}
	printStatus(reports, pc.Root)

	totalMissing := 0
	for _, r := range reports {
		totalMissing += len(r.Missing)
	}
	if totalMissing == 0 {
		fmt.Println("\n" + ui.Success.Render("✓ All translations are complete!"))
		return nil
	}

	if flagDryRun {
		printDryRun(reports, pc)
		return nil
	}

	// Select the catalog(s) to translate.
	selected, err := selectMissing(reports, pc.Root)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return nil
	}

	// Narrow down to a batch.
	batch, err := selectBatch(selected, pc.Config.BaseLanguages)
	if err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}

	// API key and model are only needed once we actually translate.
	store := keychain.New()
	globalSettings, err := settings.Load()
	if err != nil {
		return err
	}
	apiKey, err := resolveAPIKey(store, globalSettings, flagKey)
	if err != nil {
		return err
	}
	modelID, err := resolveModel(context.Background(), apiKey, pc.Config, globalSettings)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s %d translations in %d keys · model %s\n",
		ui.Title.Render("Translating"), len(batch), len(uniqueKeys(batch)), ui.Accent.Render(modelID))
	fmt.Println(ui.Dim.Render("Press Ctrl+C to stop — finished translations can still be saved."))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := anthropic.New(apiKey, modelID)
	opts := translate.Options{
		Template:            pc.Config.CustomPrompt,
		UntranslatableWords: pc.Config.UntranslatableWords,
		FormalLanguages:     pc.Config.FormalLanguages,
	}

	results := runTranslations(ctx, client, batch, opts)
	stop()

	// Offer a retry round for failures (skipping cancelled items).
	results = retryFailures(results, client, opts)

	ok, failed := splitResults(results)
	printRunSummary(ok, failed)
	if len(ok) == 0 {
		return fmt.Errorf("no translations to save")
	}

	// Review warnings for brand words that got translated anyway.
	printViolationWarnings(ok, pc.Config.UntranslatableWords)

	var save bool
	err = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Save %d translations to %d file(s)?", len(ok), len(filesOf(ok)))).
			Value(&save),
	)).Run()
	if err != nil {
		return err
	}
	if !save {
		fmt.Println(ui.Dim.Render("Nothing saved."))
		return nil
	}

	return writeResults(ok, pc.Root)
}

// runTranslations executes the worker pool with live per-item output.
func runTranslations(ctx context.Context, client *anthropic.Client, batch []analyze.Missing, opts translate.Options) []translate.Result {
	done := 0
	total := len(batch)
	counterWidth := len(fmt.Sprint(total))

	langs := make([]string, len(batch))
	for i, m := range batch {
		langs[i] = m.TargetLanguage
	}
	langWidth := ui.MaxLangWidth(langs)

	return translate.Run(ctx, batch, 4, func(ctx context.Context, m analyze.Missing) (string, error) {
		raw, err := client.Complete(ctx, translate.BuildPrompt(m, opts))
		if err != nil {
			return "", err
		}
		return translate.CleanTranslation(raw), nil
	}, func(r translate.Result) {
		done++
		prefix := ui.Dim.Render(fmt.Sprintf("[%*d/%d]", counterWidth, done, total))
		lang := ui.LangPadded(r.Missing.TargetLanguage, langWidth)
		if r.Err != nil {
			if ctx.Err() != nil {
				return // don't spam lines for cancelled leftovers
			}
			fmt.Printf("%s %s %s  %s\n", prefix, lang, r.Missing.Key, ui.Error.Render("✗ "+r.Err.Error()))
			return
		}
		fmt.Printf("%s %s %s %s\n", prefix, lang, ui.Dim.Render(r.Missing.Key+" →"), r.Translation)
	})
}

// retryFailures offers one interactive retry round for failed translations.
func retryFailures(results []translate.Result, client *anthropic.Client, opts translate.Options) []translate.Result {
	var failedIdx []int
	for i, r := range results {
		if r.Err != nil && r.Err != context.Canceled {
			failedIdx = append(failedIdx, i)
		}
	}
	if len(failedIdx) == 0 {
		return results
	}

	var retry bool
	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("%d translations failed. Retry them?", len(failedIdx))).
			Value(&retry),
	)).Run()
	if err != nil || !retry {
		return results
	}

	var toRetry []analyze.Missing
	for _, i := range failedIdx {
		toRetry = append(toRetry, results[i].Missing)
	}

	retried := runTranslations(context.Background(), client, toRetry, opts)
	for n, i := range failedIdx {
		results[i] = retried[n]
	}
	return results
}

func splitResults(results []translate.Result) (ok, failed []translate.Result) {
	for _, r := range results {
		if r.Err == nil {
			ok = append(ok, r)
		} else {
			failed = append(failed, r)
		}
	}
	return ok, failed
}

func printRunSummary(ok, failed []translate.Result) {
	fmt.Println()
	if len(failed) > 0 {
		fmt.Println(ui.Warn.Render(fmt.Sprintf("⚠ %d done, %d failed or skipped", len(ok), len(failed))))
	} else {
		fmt.Println(ui.Success.Render(fmt.Sprintf("✓ All %d translations done", len(ok))))
	}
}

func printViolationWarnings(ok []translate.Result, untranslatable []string) {
	if len(untranslatable) == 0 {
		return
	}
	violations := map[string][]string{} // word -> keys
	for _, r := range ok {
		for _, word := range analyze.CheckUntranslatableWords(r.Missing.SourceText, r.Translation, untranslatable) {
			violations[word] = append(violations[word], r.Missing.Key)
		}
	}
	for word, keys := range violations {
		fmt.Println(ui.Warn.Render(fmt.Sprintf("⚠ %q may have been translated in: %s", word, strings.Join(dedupe(keys), ", "))))
	}
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range items {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func filesOf(results []translate.Result) map[string][]translate.Result {
	byFile := map[string][]translate.Result{}
	for _, r := range results {
		byFile[r.Missing.FilePath] = append(byFile[r.Missing.FilePath], r)
	}
	return byFile
}

// writeResults applies the successful translations to their catalogs, one
// load/save per file.
func writeResults(ok []translate.Result, root string) error {
	byFile := filesOf(ok)

	paths := make([]string, 0, len(byFile))
	for path := range byFile {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		catalog, err := xcstrings.Load(path)
		if err != nil {
			return err
		}

		for _, r := range byFile[path] {
			if r.Missing.IsPlural {
				one, other := translate.ParsePluralResponse(r.Translation)
				err = catalog.SetPluralTranslation(r.Missing.Key, r.Missing.TargetLanguage, one, other)
			} else {
				err = catalog.SetTranslation(r.Missing.Key, r.Missing.TargetLanguage, r.Translation)
			}
			if err != nil {
				return err
			}
		}

		if err := catalog.Save(path); err != nil {
			return err
		}
		fmt.Printf("%s %s (%d translations)\n", ui.Success.Render("✓ saved"), displayPath(root, path), len(byFile[path]))
	}
	return nil
}

// selectMissing lets the user pick one catalog or all of them.
func selectMissing(reports []*analyze.Report, root string) ([]analyze.Missing, error) {
	var withMissing []*analyze.Report
	for _, r := range reports {
		if len(r.Missing) > 0 {
			withMissing = append(withMissing, r)
		}
	}

	if len(withMissing) == 1 {
		return withMissing[0].Missing, nil
	}

	const allFiles = -1
	options := []huh.Option[int]{}
	for i, r := range withMissing {
		label := fmt.Sprintf("%s  %s", displayPath(root, r.FilePath),
			ui.Error.Render(fmt.Sprintf("%d missing", len(r.Missing))))
		options = append(options, huh.NewOption(label, i))
	}
	total := 0
	for _, r := range withMissing {
		total += len(r.Missing)
	}
	options = append(options, huh.NewOption(fmt.Sprintf("All files  %s", ui.Error.Render(fmt.Sprintf("%d missing", total))), allFiles))

	var picked int
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[int]().
			Title("Which catalog do you want to translate?").
			Options(options...).
			Value(&picked),
	)).Run()
	if err != nil {
		return nil, err
	}

	if picked == allFiles {
		var all []analyze.Missing
		for _, r := range withMissing {
			all = append(all, r.Missing...)
		}
		return all, nil
	}
	return withMissing[picked].Missing, nil
}

// selectBatch narrows the selection to a manageable batch of keys.
func selectBatch(selected []analyze.Missing, baseLanguages []string) ([]analyze.Missing, error) {
	keys := uniqueKeys(selected)

	type batchChoice struct {
		limit    int // 0 = all
		baseOnly bool
	}

	options := []huh.Option[batchChoice]{
		huh.NewOption(fmt.Sprintf("All keys (%d)", len(keys)), batchChoice{}),
	}
	for _, n := range []int{3, 10, 20} {
		if len(keys) > n {
			options = append(options, huh.NewOption(fmt.Sprintf("First %d keys", n), batchChoice{limit: n}))
		}
	}
	if len(baseLanguages) > 0 {
		options = append(options, huh.NewOption(
			fmt.Sprintf("Base languages only (%s)", strings.Join(baseLanguages, ", ")),
			batchChoice{baseOnly: true}))
	}

	if len(options) == 1 {
		return selected, nil
	}

	var choice batchChoice
	err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[batchChoice]().
			Title("How much do you want to translate in this run?").
			Options(options...).
			Value(&choice),
	)).Run()
	if err != nil {
		return nil, err
	}

	if choice.baseOnly {
		var filtered []analyze.Missing
		for _, m := range selected {
			for _, lang := range baseLanguages {
				if m.TargetLanguage == lang {
					filtered = append(filtered, m)
					break
				}
			}
		}
		return filtered, nil
	}

	if choice.limit > 0 && choice.limit < len(keys) {
		allowed := map[string]bool{}
		for _, key := range keys[:choice.limit] {
			allowed[key] = true
		}
		var filtered []analyze.Missing
		for _, m := range selected {
			if allowed[m.Key] {
				filtered = append(filtered, m)
			}
		}
		return filtered, nil
	}

	return selected, nil
}

func printDryRun(reports []*analyze.Report, pc *projectContext) {
	fmt.Println("\n" + ui.Title.Render("Dry run — these translations would be requested:"))
	for _, r := range reports {
		if len(r.Missing) == 0 {
			continue
		}
		fmt.Printf("\n%s\n", ui.Title.Render(displayPath(pc.Root, r.FilePath)))
		langWidth := ui.MaxLangWidth(r.TargetLanguages)
		for _, m := range r.Missing {
			fmt.Printf("  %s %s %s\n", ui.LangPadded(m.TargetLanguage, langWidth), m.Key, ui.Dim.Render("← "+m.SourceText))
		}
	}
}
