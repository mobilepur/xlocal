package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/project"
	"github.com/MobilePur/xlocal/internal/settings"
)

func TestResolveClientsUsesOverrideModel(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, `{"targetLanguages":["de"],"model":"claude-root"}`)
	mergedDir := filepath.Join(root, "Merged")
	writeTestConfig(t, mergedDir, `{"model":"claude-ignored"}`)
	overrideDir := filepath.Join(root, "Standalone")
	writeTestConfig(t, overrideDir, `{"strategy":"override","targetLanguages":["ja"],"model":"claude-sub"}`)

	resolver, err := project.NewConfigResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	pc := &projectContext{Root: root, Resolver: resolver}
	rootCatalog := filepath.Join(root, "Root.xcstrings")
	mergedCatalog := filepath.Join(mergedDir, "Merged.xcstrings")
	overrideCatalog := filepath.Join(overrideDir, "Standalone.xcstrings")
	batch := []analyze.Missing{
		{FilePath: rootCatalog},
		{FilePath: mergedCatalog},
		{FilePath: overrideCatalog},
	}

	clients, modelIDs, err := resolveClients(context.Background(), "key", batch, pc, &settings.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if clients[rootCatalog].Model != "claude-root" || clients[mergedCatalog].Model != "claude-root" {
		t.Errorf("merge config changed inherited model: root=%q merged=%q", clients[rootCatalog].Model, clients[mergedCatalog].Model)
	}
	if clients[overrideCatalog].Model != "claude-sub" {
		t.Errorf("override model = %q, want claude-sub", clients[overrideCatalog].Model)
	}
	if len(modelIDs) != 2 {
		t.Errorf("modelIDs = %v, want two models", modelIDs)
	}
}

func writeTestConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, project.ConfigFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
