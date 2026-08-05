package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MobilePur/xlocal/internal/project"
)

func TestCreateConfigSkeletonNestedIsPartialMerge(t *testing.T) {
	root := t.TempDir()
	rootConfig := []byte(`{"targetLanguages":["de"],"formalLanguages":["de"]}`)
	if err := os.WriteFile(filepath.Join(root, project.ConfigFileName), rootConfig, 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "Feature")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := createConfigSkeleton(nested); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(nested, project.ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["strategy"] != "merge" {
		t.Errorf("strategy = %v, want merge", got["strategy"])
	}
	for _, field := range []string{"targetLanguages", "baseLanguages", "untranslatableWords", "formalLanguages", "model", "exclude", "excludeKeys", "customPrompt"} {
		if _, exists := got[field]; exists {
			t.Errorf("empty nested skeleton unexpectedly contains %s", field)
		}
	}
}
