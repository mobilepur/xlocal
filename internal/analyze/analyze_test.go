package analyze

import (
	"reflect"
	"testing"

	"github.com/MobilePur/xlocal/internal/xcstrings"
)

func stringUnit(value string) xcstrings.LocalizationEntry {
	return xcstrings.LocalizationEntry{
		StringUnit: &xcstrings.StringUnit{State: xcstrings.StateTranslated, Value: value},
	}
}

func plural(one, other string) xcstrings.LocalizationEntry {
	return xcstrings.LocalizationEntry{
		Variations: &xcstrings.PluralVariations{
			Plural: map[string]xcstrings.PluralCase{
				"one":   {StringUnit: xcstrings.StringUnit{State: xcstrings.StateTranslated, Value: one}},
				"other": {StringUnit: xcstrings.StringUnit{State: xcstrings.StateTranslated, Value: other}},
			},
		},
	}
}

func catalog(strings map[string]xcstrings.StringEntry) *xcstrings.File {
	return &xcstrings.File{SourceLanguage: "en", Strings: strings, Version: "1.0"}
}

func TestMissingDetection(t *testing.T) {
	tests := []struct {
		name    string
		entry   xcstrings.StringEntry
		targets []string
		want    []string // languages reported missing
	}{
		{
			name: "language completely absent",
			entry: xcstrings.StringEntry{Localizations: map[string]xcstrings.LocalizationEntry{
				"en": stringUnit("Hello"),
			}},
			targets: []string{"en", "de"},
			want:    []string{"de"},
		},
		{
			name: "empty value counts as missing",
			entry: xcstrings.StringEntry{Localizations: map[string]xcstrings.LocalizationEntry{
				"en": stringUnit("Hello"),
				"de": stringUnit(""),
			}},
			targets: []string{"en", "de"},
			want:    []string{"de"},
		},
		{
			name: "whitespace-only value counts as missing",
			entry: xcstrings.StringEntry{Localizations: map[string]xcstrings.LocalizationEntry{
				"en": stringUnit("Hello"),
				"de": stringUnit("   "),
			}},
			targets: []string{"en", "de"},
			want:    []string{"de"},
		},
		{
			name: "complete translation is not missing",
			entry: xcstrings.StringEntry{Localizations: map[string]xcstrings.LocalizationEntry{
				"en": stringUnit("Hello"),
				"de": stringUnit("Hallo"),
			}},
			targets: []string{"en", "de"},
			want:    nil,
		},
		{
			name: "no localizations at all",
			entry: xcstrings.StringEntry{Comment: "Greeting"},
			targets: []string{"en", "de"},
			want:    []string{"en", "de"},
		},
		{
			name: "plural with only other form is missing",
			entry: xcstrings.StringEntry{Localizations: map[string]xcstrings.LocalizationEntry{
				"en": plural("1 page", "%lld pages"),
				"de": {Variations: &xcstrings.PluralVariations{
					Plural: map[string]xcstrings.PluralCase{
						"other": {StringUnit: xcstrings.StringUnit{Value: "%lld Seiten"}},
					},
				}},
			}},
			targets: []string{"en", "de"},
			want:    []string{"de"},
		},
		{
			name: "plural with one and other is complete",
			entry: xcstrings.StringEntry{Localizations: map[string]xcstrings.LocalizationEntry{
				"en": plural("1 page", "%lld pages"),
				"de": plural("1 Seite", "%lld Seiten"),
			}},
			targets: []string{"en", "de"},
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := File("test.xcstrings", catalog(map[string]xcstrings.StringEntry{"key": tt.entry}), tt.targets)

			var got []string
			for _, m := range report.Missing {
				got = append(got, m.TargetLanguage)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("missing languages = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSourceTextFallbackChain(t *testing.T) {
	tests := []struct {
		name  string
		entry xcstrings.StringEntry
		want  string
	}{
		{
			name: "source language value wins",
			entry: xcstrings.StringEntry{
				Comment: "A comment",
				Localizations: map[string]xcstrings.LocalizationEntry{
					"en": stringUnit("Hello"),
				},
			},
			want: "Hello",
		},
		{
			name: "plural source uses other form",
			entry: xcstrings.StringEntry{
				Localizations: map[string]xcstrings.LocalizationEntry{
					"en": plural("1 page", "%lld pages"),
				},
			},
			want: "%lld pages",
		},
		{
			name:  "comment when no source translation",
			entry: xcstrings.StringEntry{Comment: "A comment"},
			want:  "A comment",
		},
		{
			name:  "key as last resort",
			entry: xcstrings.StringEntry{},
			want:  "greetingKey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := File("test.xcstrings", catalog(map[string]xcstrings.StringEntry{"greetingKey": tt.entry}), []string{"de"})
			if len(report.Missing) == 0 {
				t.Fatal("expected missing translations")
			}
			if got := report.Missing[0].SourceText; got != tt.want {
				t.Errorf("SourceText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExistingTranslationsContext(t *testing.T) {
	entry := xcstrings.StringEntry{
		Localizations: map[string]xcstrings.LocalizationEntry{
			"en": stringUnit("Hello"),
			"fr": stringUnit("Bonjour"),
			"nl": stringUnit(""),
			"es": plural("1 hola", "%lld holas"),
		},
	}
	report := File("test.xcstrings", catalog(map[string]xcstrings.StringEntry{"key": entry}), []string{"de"})

	if len(report.Missing) != 1 {
		t.Fatalf("expected 1 missing, got %d", len(report.Missing))
	}
	existing := report.Missing[0].Existing

	want := map[string]string{
		"en": "Hello",
		"fr": "Bonjour",
		"es": "%lld holas", // plural context uses the other form
	}
	if !reflect.DeepEqual(existing, want) {
		t.Errorf("Existing = %v, want %v", existing, want)
	}
}

func TestExistingDropsUntranslatedSourceLanguage(t *testing.T) {
	// When the source language has no real translation, the key/comment
	// fallback must not leak into the context map as an "en translation".
	entry := xcstrings.StringEntry{
		Comment: "Greeting",
		Localizations: map[string]xcstrings.LocalizationEntry{
			"fr": stringUnit("Bonjour"),
		},
	}
	report := File("test.xcstrings", catalog(map[string]xcstrings.StringEntry{"key": entry}), []string{"de"})

	existing := report.Missing[0].Existing
	if _, ok := existing["en"]; ok {
		t.Errorf("untranslated source language leaked into context: %v", existing)
	}
	if existing["fr"] != "Bonjour" {
		t.Errorf("fr context missing: %v", existing)
	}
}

func TestIsPluralFlag(t *testing.T) {
	entries := map[string]xcstrings.StringEntry{
		"pages": {Localizations: map[string]xcstrings.LocalizationEntry{
			"en": plural("1 page", "%lld pages"),
		}},
		"hello": {Localizations: map[string]xcstrings.LocalizationEntry{
			"en": stringUnit("Hello"),
		}},
	}
	report := File("test.xcstrings", catalog(entries), []string{"de"})

	byKey := map[string]bool{}
	for _, m := range report.Missing {
		byKey[m.Key] = m.IsPlural
	}
	if !byKey["pages"] {
		t.Error("pages should be flagged plural")
	}
	if byKey["hello"] {
		t.Error("hello should not be flagged plural")
	}
}

func TestDeterministicOrder(t *testing.T) {
	entries := map[string]xcstrings.StringEntry{
		"zebra": {}, "apple": {}, "Mango": {},
	}
	report := File("test.xcstrings", catalog(entries), []string{"de", "fr"})

	var got []string
	for _, m := range report.Missing {
		got = append(got, m.Key+":"+m.TargetLanguage)
	}
	want := []string{"apple:de", "apple:fr", "Mango:de", "Mango:fr", "zebra:de", "zebra:fr"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestReportCounts(t *testing.T) {
	entries := map[string]xcstrings.StringEntry{
		"a": {Localizations: map[string]xcstrings.LocalizationEntry{"en": stringUnit("A"), "de": stringUnit("A")}},
		"b": {Localizations: map[string]xcstrings.LocalizationEntry{"en": stringUnit("B")}},
	}
	report := File("test.xcstrings", catalog(entries), []string{"en", "de"})

	if report.TotalStrings != 2 {
		t.Errorf("TotalStrings = %d, want 2", report.TotalStrings)
	}
	if len(report.Missing) != 1 {
		t.Errorf("len(Missing) = %d, want 1", len(report.Missing))
	}
	if report.FilePath != "test.xcstrings" {
		t.Errorf("FilePath = %q", report.FilePath)
	}
}

func TestCheckUntranslatableWords(t *testing.T) {
	words := []string{"Workspaces", "Pitchview"}

	tests := []struct {
		name        string
		source      string
		translation string
		want        []string
	}{
		{
			name:        "preserved word passes",
			source:      "Open Workspaces",
			translation: "Workspaces öffnen",
			want:        nil,
		},
		{
			name:        "translated brand word is flagged",
			source:      "Open Workspaces",
			translation: "Arbeitsbereiche öffnen",
			want:        []string{"Workspaces"},
		},
		{
			name:        "case-insensitive match counts as preserved",
			source:      "Open Workspaces",
			translation: "öffne workspaces",
			want:        nil,
		},
		{
			name:        "word absent from source is ignored",
			source:      "Hello",
			translation: "Hallo",
			want:        nil,
		},
		{
			name:        "multiple violations",
			source:      "Pitchview Workspaces",
			translation: "Präsentationsansicht Arbeitsbereiche",
			want:        []string{"Workspaces", "Pitchview"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckUntranslatableWords(tt.source, tt.translation, words)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("violations = %v, want %v", got, tt.want)
			}
		})
	}
}
