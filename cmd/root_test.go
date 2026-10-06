package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/anthropic"
	"github.com/MobilePur/xlocal/internal/translate"
)

func TestRunTranslationsValidatesPluralPlaceholders(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response string
		valid    bool
	}{
		{"preserves int", "one: %d Datei | other: %d Dateien", true},
		{"allows source-backed omission", "one: Eine Datei | other: %d Dateien", true},
		{"rejects changed type", "one: %lld Datei | other: %lld Dateien", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"content": []map[string]string{{"type": "text", "text": tt.response}},
				})
			}))
			defer server.Close()
			client := anthropic.New("test-key", "test-model")
			client.BaseURL = server.URL
			batch := []analyze.Missing{{
				Key: "files", SourceText: "%d files", TargetLanguage: "de", IsPlural: true,
				SourcePluralForms: map[string]string{"one": "One file", "other": "%d files"},
			}}
			results := runTranslations(context.Background(), func(path string) *anthropic.Client {
				return client
			}, batch, func(path string) translate.Options {
				return translate.Options{}
			})
			ok, failed := splitResults(results)
			if tt.valid {
				if len(ok) != 1 || len(failed) != 0 || ok[0].Translation != tt.response {
					t.Fatalf("valid response not available for saving: %+v", results)
				}
			} else if len(ok) != 0 || len(failed) != 1 {
				t.Fatalf("invalid response available for saving: %+v", results)
			}
		})
	}
}
