package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/anthropic"
	"github.com/MobilePur/xlocal/internal/llm"
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

	clients, modelIDs, err := resolveClients(context.Background(), func() (string, error) { return "key", nil }, batch, pc, &settings.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if modelOf(clients[rootCatalog]) != "claude-root" || modelOf(clients[mergedCatalog]) != "claude-root" {
		t.Errorf("merge config changed inherited model: root=%q merged=%q", modelOf(clients[rootCatalog]), modelOf(clients[mergedCatalog]))
	}
	if modelOf(clients[overrideCatalog]) != "claude-sub" {
		t.Errorf("override model = %q, want claude-sub", modelOf(clients[overrideCatalog]))
	}
	if len(modelIDs) != 2 {
		t.Errorf("modelIDs = %v, want two models", modelIDs)
	}
}

func modelOf(c llm.Completer) string {
	if a, ok := c.(*anthropic.Client); ok {
		return a.Model
	}
	return ""
}

func TestResolveClientsSkipsAPIKeyForCLIProviders(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	root := t.TempDir()
	writeTestConfig(t, root, `{"targetLanguages":["de"],"provider":"claude-cli","model":"haiku"}`)
	sub := filepath.Join(root, "Sub")
	writeTestConfig(t, sub, `{"strategy":"override","targetLanguages":["ja"],"provider":"codex-cli","model":"sonnet"}`)

	resolver, err := project.NewConfigResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	pc := &projectContext{Root: root, Resolver: resolver}
	rootCatalog := filepath.Join(root, "Root.xcstrings")
	subCatalog := filepath.Join(sub, "Sub.xcstrings")
	batch := []analyze.Missing{{FilePath: rootCatalog}, {FilePath: subCatalog}}

	getKey := func() (string, error) {
		t.Error("API key requested although no catalog uses the Anthropic API")
		return "", nil
	}
	clients, labels, err := resolveClients(context.Background(), getKey, batch, pc, &settings.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := clients[rootCatalog].(*llm.Claude); !ok || c.Model != "haiku" {
		t.Errorf("root client = %#v, want Claude with model haiku", clients[rootCatalog])
	}
	// Anthropic aliases mean nothing to Codex, so it falls back to its default.
	if c, ok := clients[subCatalog].(*llm.Codex); !ok || c.Model != "" {
		t.Errorf("sub client = %#v, want Codex with default model", clients[subCatalog])
	}
	if len(labels) != 2 || labels[0] != "claude-cli:haiku" || labels[1] != "codex-cli" {
		t.Errorf("labels = %v", labels)
	}
}

func TestResolveClientsRejectsUnknownProvider(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, root, `{"targetLanguages":["de"],"provider":"gemini"}`)
	resolver, err := project.NewConfigResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	pc := &projectContext{Root: root, Resolver: resolver}
	batch := []analyze.Missing{{FilePath: filepath.Join(root, "Root.xcstrings")}}
	_, _, err = resolveClients(context.Background(), func() (string, error) { return "key", nil }, batch, pc, &settings.Settings{})
	if err == nil {
		t.Fatal("expected an error for an unknown provider")
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
