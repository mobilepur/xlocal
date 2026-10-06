package android

import (
	"slices"
	"testing"
)

func TestPluralCategories(t *testing.T) {
	tests := []struct {
		language string
		want     []string
	}{
		{"en", []string{"one", "other"}},
		{"de-DE", []string{"one", "other"}},
		{"fr", []string{"one", "many", "other"}},
		{"es-MX", []string{"one", "many", "other"}},
		{"it", []string{"one", "many", "other"}},
		{"pt-PT", []string{"one", "many", "other"}},
		{"pt-BR", []string{"one", "many", "other"}},
		{"ru", []string{"one", "few", "many", "other"}},
		{"uk", []string{"one", "few", "many", "other"}},
		{"pl", []string{"one", "few", "many", "other"}},
		{"cs", []string{"one", "few", "other"}},
		{"sk", []string{"one", "few", "other"}},
		{"lt", []string{"one", "few", "other"}},
		{"ar-EG", []string{"zero", "one", "two", "few", "many", "other"}},
		{"he", []string{"one", "two", "other"}},
		{"iw", []string{"one", "two", "other"}},
		{"ro", []string{"one", "few", "other"}},
		{"sr-Latn", []string{"one", "few", "other"}},
		{"sl", []string{"one", "two", "few", "other"}},
		{"lv", []string{"zero", "one", "other"}},
		{"ja", []string{"other"}},
		{"ko", []string{"other"}},
		{"zh-Hant-TW", []string{"other"}},
		{"id", []string{"other"}},
		{"th", []string{"other"}},
		{"vi", []string{"other"}},
		{"tr", []string{"one", "other"}},
		{"nl", []string{"one", "other"}},
		{"sv", []string{"one", "other"}},
	}
	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			got, err := PluralCategories(tt.language)
			if err != nil || !slices.Equal(got, tt.want) {
				t.Fatalf("PluralCategories(%q)=%v,%v; want %v", tt.language, got, err, tt.want)
			}
		})
	}
}

func TestPluralCategoriesRejectUnknown(t *testing.T) {
	for _, language := range []string{"", "und", "not a locale", "zz", "cy"} {
		if got, err := PluralCategories(language); err == nil {
			t.Errorf("PluralCategories(%q)=%v; want unsupported language error", language, got)
		}
	}
}

func TestPluralCategoriesReturnsIndependentSlice(t *testing.T) {
	got, err := PluralCategories("en")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("empty categories")
	}
	got[0] = "changed"
	again, err := PluralCategories("en")
	if err != nil || again[0] != "one" {
		t.Fatalf("table mutated: %v,%v", again, err)
	}
}
