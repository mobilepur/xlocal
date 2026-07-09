package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// touch creates an empty file including parent directories.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeConfig(t *testing.T, dir string, content string) {
	t.Helper()
	mkdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validConfig = `{
  "targetLanguages": ["en", "de", "fr"],
  "baseLanguages": ["en"],
  "untranslatableWords": ["MyBrand"],
  "formalLanguages": ["fr"],
  "model": "sonnet",
  "exclude": ["Vendor"]
}`

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, validConfig)

	cfg, err := LoadConfig(filepath.Join(dir, ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(cfg.TargetLanguages, []string{"en", "de", "fr"}) {
		t.Errorf("TargetLanguages = %v", cfg.TargetLanguages)
	}
	if !reflect.DeepEqual(cfg.BaseLanguages, []string{"en"}) {
		t.Errorf("BaseLanguages = %v", cfg.BaseLanguages)
	}
	if cfg.Model != "sonnet" {
		t.Errorf("Model = %q", cfg.Model)
	}
	if !reflect.DeepEqual(cfg.Exclude, []string{"Vendor"}) {
		t.Errorf("Exclude = %v", cfg.Exclude)
	}
}

func TestLoadConfigRequiresTargetLanguages(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"targetLanguages": []}`)

	if _, err := LoadConfig(filepath.Join(dir, ConfigFileName)); err == nil {
		t.Error("expected error for empty targetLanguages")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{not json`)

	if _, err := LoadConfig(filepath.Join(dir, ConfigFileName)); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestConfigSaveRoundTrip(t *testing.T) {
	cfg := &Config{
		TargetLanguages: []string{"en", "de"},
		BaseLanguages:   []string{"en"},
	}
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.TargetLanguages, cfg.TargetLanguages) {
		t.Errorf("round trip lost targetLanguages: %v", loaded.TargetLanguages)
	}
}

func TestFindConfigUpwards(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, validConfig)
	nested := filepath.Join(root, "App", "Sources", "Feature")
	mkdir(t, nested)

	found, ok := FindConfigUpwards(nested)
	if !ok {
		t.Fatal("config not found from nested dir")
	}
	if found != filepath.Join(root, ConfigFileName) {
		t.Errorf("found %q, want config in %q", found, root)
	}

	// Also finds it when starting directly in the config dir.
	found, ok = FindConfigUpwards(root)
	if !ok || found != filepath.Join(root, ConfigFileName) {
		t.Errorf("not found from config dir itself: %q, %v", found, ok)
	}
}

func TestFindConfigUpwardsNotFound(t *testing.T) {
	dir := t.TempDir()
	if _, ok := FindConfigUpwards(dir); ok {
		t.Error("unexpectedly found a config in empty temp dir")
	}
}

func TestFindTopmostConfig(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, validConfig)
	writeConfig(t, filepath.Join(root, "Widget"), `{"targetLanguages": ["en"]}`)
	nested := filepath.Join(root, "Widget", "Sources")
	mkdir(t, nested)

	// From deep inside, the topmost (root) config anchors the project — not
	// the nearer Widget config that FindConfigUpwards would return.
	found, ok := FindTopmostConfig(nested)
	if !ok {
		t.Fatal("no config found")
	}
	if found != filepath.Join(root, ConfigFileName) {
		t.Errorf("found %q, want the root config", found)
	}
}

func TestMerge(t *testing.T) {
	base := &Config{
		TargetLanguages:     []string{"en", "de", "fr"},
		BaseLanguages:       []string{"en"},
		UntranslatableWords: []string{"Brand"},
		FormalLanguages:     []string{"fr"},
		ExcludeKeys:         []string{"a"},
		CustomPrompt:        "base prompt",
		Model:               "opus",
	}
	override := &Config{
		TargetLanguages:     []string{"en", "de"}, // overrides
		UntranslatableWords: []string{"Widget"},   // union
		ExcludeKeys:         []string{"b"},        // union
		Model:               "haiku",              // ignored — model stays global
	}

	got := Merge(base, override)

	if !reflect.DeepEqual(got.TargetLanguages, []string{"en", "de"}) {
		t.Errorf("TargetLanguages = %v, want override to win", got.TargetLanguages)
	}
	if !reflect.DeepEqual(got.FormalLanguages, []string{"fr"}) {
		t.Errorf("FormalLanguages = %v, want inherited", got.FormalLanguages)
	}
	if got.CustomPrompt != "base prompt" {
		t.Errorf("CustomPrompt = %q, want inherited", got.CustomPrompt)
	}
	if got.Model != "opus" {
		t.Errorf("Model = %q, want the base (global) model", got.Model)
	}
	if !reflect.DeepEqual(got.UntranslatableWords, []string{"Brand", "Widget"}) {
		t.Errorf("UntranslatableWords = %v, want union", got.UntranslatableWords)
	}
	if !reflect.DeepEqual(got.ExcludeKeys, []string{"a", "b"}) {
		t.Errorf("ExcludeKeys = %v, want union", got.ExcludeKeys)
	}

	// The base must not be mutated.
	if !reflect.DeepEqual(base.UntranslatableWords, []string{"Brand"}) {
		t.Errorf("base was mutated: %v", base.UntranslatableWords)
	}
}

func TestConfigResolver(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `{
	  "targetLanguages": ["en", "de", "fr"],
	  "untranslatableWords": ["Brand"],
	  "model": "opus"
	}`)
	writeConfig(t, filepath.Join(root, "Widget"), `{
	  "targetLanguages": ["en", "de"],
	  "untranslatableWords": ["Widget"],
	  "model": "haiku"
	}`)

	r, err := NewConfigResolver(root)
	if err != nil {
		t.Fatal(err)
	}

	rootCfg := r.Resolve(root)
	if !reflect.DeepEqual(rootCfg.TargetLanguages, []string{"en", "de", "fr"}) {
		t.Errorf("root TargetLanguages = %v", rootCfg.TargetLanguages)
	}

	widget := r.Resolve(filepath.Join(root, "Widget"))
	if !reflect.DeepEqual(widget.TargetLanguages, []string{"en", "de"}) {
		t.Errorf("widget TargetLanguages = %v, want override", widget.TargetLanguages)
	}
	if !reflect.DeepEqual(widget.UntranslatableWords, []string{"Brand", "Widget"}) {
		t.Errorf("widget UntranslatableWords = %v, want union", widget.UntranslatableWords)
	}
	if widget.Model != "opus" {
		t.Errorf("widget Model = %q, want the global root model", widget.Model)
	}

	// A directory without its own config inherits the nearest one above it.
	deep := r.Resolve(filepath.Join(root, "Widget", "Sub", "Feature"))
	if !reflect.DeepEqual(deep.TargetLanguages, []string{"en", "de"}) {
		t.Errorf("deep TargetLanguages = %v, want Widget's", deep.TargetLanguages)
	}
}

func TestConfigResolverFindCatalogsScopedExclude(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, `{"targetLanguages": ["en"]}`)
	writeConfig(t, filepath.Join(root, "Widget"), `{"exclude": ["Generated"]}`)

	touch(t, filepath.Join(root, "Widget", "Generated", "Localizable.xcstrings")) // excluded
	touch(t, filepath.Join(root, "Widget", "Localizable.xcstrings"))              // kept
	touch(t, filepath.Join(root, "App", "Generated", "Localizable.xcstrings"))    // kept: exclude is scoped to Widget

	r, err := NewConfigResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	catalogs, err := r.FindCatalogs()
	if err != nil {
		t.Fatal(err)
	}

	var rel []string
	for _, c := range catalogs {
		p, _ := filepath.Rel(root, c)
		rel = append(rel, p)
	}
	want := []string{
		filepath.Join("App", "Generated", "Localizable.xcstrings"),
		filepath.Join("Widget", "Localizable.xcstrings"),
	}
	if !reflect.DeepEqual(rel, want) {
		t.Errorf("catalogs = %v, want %v", rel, want)
	}
}

func TestDiscoverProjects(t *testing.T) {
	root := t.TempDir()

	// A project with an xlocal config.
	writeConfig(t, filepath.Join(root, "AppWithConfig"), validConfig)
	// An Xcode project without config.
	mkdir(t, filepath.Join(root, "PlainApp", "PlainApp.xcodeproj"))
	// A workspace.
	mkdir(t, filepath.Join(root, "BigApp", "BigApp.xcworkspace"))
	// A Swift package.
	touch(t, filepath.Join(root, "MyLib", "Package.swift"))
	// Noise that must be skipped.
	mkdir(t, filepath.Join(root, "PlainApp", "Pods", "Pod.xcodeproj"))
	mkdir(t, filepath.Join(root, "node_modules", "dep", "Dep.xcodeproj"))
	mkdir(t, filepath.Join(root, ".git", "Fake.xcodeproj"))
	// Too deep (beyond maxDepth 4 directories below root).
	mkdir(t, filepath.Join(root, "a", "b", "c", "d", "e", "Deep.xcodeproj"))

	candidates, err := DiscoverProjects(root, 4)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{} // dir (relative) -> HasConfig
	for _, c := range candidates {
		rel, _ := filepath.Rel(root, c.Dir)
		got[rel] = c.HasConfig
	}

	want := map[string]bool{
		"AppWithConfig": true,
		"PlainApp":      false,
		"BigApp":        false,
		"MyLib":         false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("discovered = %v, want %v", got, want)
	}

	// Configs must be listed first.
	if len(candidates) > 0 && !candidates[0].HasConfig {
		t.Errorf("first candidate should have a config: %+v", candidates[0])
	}
}

func TestDiscoverProjectsDedupes(t *testing.T) {
	// A dir with both a config and an .xcodeproj is one candidate with config.
	root := t.TempDir()
	writeConfig(t, filepath.Join(root, "App"), validConfig)
	mkdir(t, filepath.Join(root, "App", "App.xcodeproj"))

	candidates, err := DiscoverProjects(root, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1: %+v", len(candidates), candidates)
	}
	if !candidates[0].HasConfig {
		t.Error("candidate should be marked as having a config")
	}
}

func TestFindCatalogs(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "App", "Localizable.xcstrings"))
	touch(t, filepath.Join(root, "Feature", "Sub", "Other.xcstrings"))
	touch(t, filepath.Join(root, "App", "notes.txt"))
	// Default skips:
	touch(t, filepath.Join(root, "Pods", "Lib", "Localizable.xcstrings"))
	touch(t, filepath.Join(root, "DerivedData", "X", "Localizable.xcstrings"))
	touch(t, filepath.Join(root, ".build", "Y", "Localizable.xcstrings"))
	// Custom exclude:
	touch(t, filepath.Join(root, "Vendor", "Localizable.xcstrings"))

	catalogs, err := FindCatalogs(root, []string{"Vendor"})
	if err != nil {
		t.Fatal(err)
	}

	var rel []string
	for _, c := range catalogs {
		r, _ := filepath.Rel(root, c)
		rel = append(rel, r)
	}
	want := []string{
		filepath.Join("App", "Localizable.xcstrings"),
		filepath.Join("Feature", "Sub", "Other.xcstrings"),
	}
	if !reflect.DeepEqual(rel, want) {
		t.Errorf("catalogs = %v, want %v", rel, want)
	}
}

func TestFindCatalogsExcludeRelativePath(t *testing.T) {
	root := t.TempDir()
	touch(t, filepath.Join(root, "App", "Generated", "Localizable.xcstrings"))
	touch(t, filepath.Join(root, "App", "Localizable.xcstrings"))

	catalogs, err := FindCatalogs(root, []string{"App/Generated"})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalogs) != 1 {
		t.Fatalf("got %d catalogs, want 1: %v", len(catalogs), catalogs)
	}
	if filepath.Base(filepath.Dir(catalogs[0])) != "App" {
		t.Errorf("wrong catalog survived: %v", catalogs)
	}
}
