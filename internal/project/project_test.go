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
