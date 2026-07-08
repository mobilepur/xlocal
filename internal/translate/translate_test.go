package translate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MobilePur/xlocal/internal/analyze"
)

func baseMissing() analyze.Missing {
	return analyze.Missing{
		Key:            "welcome %@",
		SourceText:     "Welcome %@!",
		TargetLanguage: "de",
		Comment:        "Welcome %@!",
		Existing:       map[string]string{"fr": "Bienvenue %@!", "en": "Welcome %@!"},
	}
}

func TestBuildPromptReplacesPlaceholders(t *testing.T) {
	prompt := BuildPrompt(baseMissing(), Options{})

	for _, want := range []string{"de", "welcome %@", "Welcome %@!"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "{TARGET_LANGUAGE}") || strings.Contains(prompt, "{KEY}") || strings.Contains(prompt, "{SOURCE_TEXT}") {
		t.Error("unreplaced placeholders left in prompt")
	}
}

func TestBuildPromptPluralInstruction(t *testing.T) {
	m := baseMissing()
	m.IsPlural = true

	prompt := BuildPrompt(m, Options{})
	if !strings.Contains(prompt, "one:") || !strings.Contains(prompt, "other:") {
		t.Error("plural format instruction missing")
	}

	m.IsPlural = false
	if strings.Contains(BuildPrompt(m, Options{}), "PLURAL FORM REQUIRED") {
		t.Error("plural instruction present for non-plural string")
	}
}

// Russian must be asked for all four CLDR categories, not the source
// language's one/other pair.
func TestBuildPromptPluralCategoriesPerLanguage(t *testing.T) {
	m := baseMissing()
	m.IsPlural = true
	m.TargetLanguage = "ru"

	prompt := BuildPrompt(m, Options{})
	for _, want := range []string{"one:", "few:", "many:", "other:"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("Russian plural prompt missing %q", want)
		}
	}
}

func TestBuildPromptFormality(t *testing.T) {
	m := baseMissing()
	m.TargetLanguage = "fr"

	formal := BuildPrompt(m, Options{FormalLanguages: []string{"fr"}})
	if !strings.Contains(formal, "FORMAL") {
		t.Error("expected formal address instruction")
	}

	informal := BuildPrompt(m, Options{FormalLanguages: []string{"de"}})
	if !strings.Contains(informal, "INFORMAL") {
		t.Error("expected informal address instruction")
	}
}

func TestBuildPromptUntranslatableWords(t *testing.T) {
	prompt := BuildPrompt(baseMissing(), Options{UntranslatableWords: []string{"Pitchview", "Workspaces"}})
	if !strings.Contains(prompt, "Pitchview") || !strings.Contains(prompt, "Workspaces") {
		t.Error("untranslatable words missing from prompt")
	}
}

func TestBuildPromptContextBeforeInstructions(t *testing.T) {
	prompt := BuildPrompt(baseMissing(), Options{})

	instructions := strings.Index(prompt, "INSTRUCTIONS:")
	existing := strings.Index(prompt, "Bienvenue %@!")
	if instructions == -1 || existing == -1 {
		t.Fatalf("prompt incomplete:\n%s", prompt)
	}
	if existing > instructions {
		t.Error("existing translations should appear before the INSTRUCTIONS section")
	}

	// Languages sorted for stable prompts: en before fr.
	if strings.Index(prompt, "EN: Welcome") > strings.Index(prompt, "FR: Bienvenue") {
		t.Error("existing translations not sorted by language")
	}
}

func TestBuildPromptCustomTemplate(t *testing.T) {
	prompt := BuildPrompt(baseMissing(), Options{Template: "Translate {SOURCE_TEXT} to {TARGET_LANGUAGE}"})
	if !strings.HasPrefix(prompt, "Translate Welcome %@! to de") {
		t.Errorf("custom template not used: %q", prompt)
	}
}

func TestParsePluralForms(t *testing.T) {
	tests := []struct {
		in         string
		categories []string
		want       map[string]string
	}{
		{"one: 1 Seite | other: %lld Seiten", []string{"one", "other"},
			map[string]string{"one": "1 Seite", "other": "%lld Seiten"}},
		{"  one: 1 page |  other: %lld pages ", []string{"one", "other"},
			map[string]string{"one": "1 page", "other": "%lld pages"}},
		{"one: %lld урок | few: %lld урока | many: %lld уроков | other: %lld урока", []string{"one", "few", "many", "other"},
			map[string]string{"one": "%lld урок", "few": "%lld урока", "many": "%lld уроков", "other": "%lld урока"}},
		// Single-category language: a plain response counts as that category.
		{"%lld語", []string{"other"},
			map[string]string{"other": "%lld語"}},
		// Ignored format with several categories: nothing parsed, caller retries.
		{"%lld Seiten", []string{"one", "other"},
			map[string]string{}},
	}

	for _, tt := range tests {
		got := ParsePluralForms(tt.in, tt.categories)
		if len(got) != len(tt.want) {
			t.Errorf("ParsePluralForms(%q) = %v; want %v", tt.in, got, tt.want)
			continue
		}
		for category, want := range tt.want {
			if got[category] != want {
				t.Errorf("ParsePluralForms(%q)[%s] = %q; want %q", tt.in, category, got[category], want)
			}
		}
	}
}

func TestValidatePluralForms(t *testing.T) {
	if err := ValidatePluralForms("one: %lld урок | few: %lld урока | many: %lld уроков | other: %lld урока", "ru"); err != nil {
		t.Errorf("complete Russian response rejected: %v", err)
	}
	if err := ValidatePluralForms("one: Изучено %lld слово | other: Изучено %lld слов", "ru"); err == nil {
		t.Error("Russian response without few/many accepted")
	}
	if err := ValidatePluralForms("", "ru"); err == nil {
		t.Error("empty response accepted")
	}
	if err := ValidatePluralForms("%lld語", "ja"); err != nil {
		t.Errorf("plain single-category response rejected: %v", err)
	}
}

func TestCleanTranslation(t *testing.T) {
	tests := []struct{ in, want string }{
		{`"Hallo"`, "Hallo"},
		{"  Hallo  ", "Hallo"},
		{`"Sag "Hi""`, `Sag "Hi"`},
		{"Hallo", "Hallo"},
	}
	for _, tt := range tests {
		if got := CleanTranslation(tt.in); got != tt.want {
			t.Errorf("CleanTranslation(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func missings(n int) []analyze.Missing {
	var out []analyze.Missing
	for i := 0; i < n; i++ {
		out = append(out, analyze.Missing{Key: fmt.Sprintf("key%02d", i), TargetLanguage: "de"})
	}
	return out
}

func TestRunTranslatesAllPreservingOrder(t *testing.T) {
	items := missings(10)

	results := Run(context.Background(), items, 4, func(ctx context.Context, m analyze.Missing) (string, error) {
		return "t:" + m.Key, nil
	}, nil)

	if len(results) != len(items) {
		t.Fatalf("got %d results, want %d", len(results), len(items))
	}
	for i, r := range results {
		if r.Missing.Key != items[i].Key {
			t.Errorf("result %d out of order: %s", i, r.Missing.Key)
		}
		if r.Translation != "t:"+items[i].Key || r.Err != nil {
			t.Errorf("result %d wrong: %+v", i, r)
		}
	}
}

func TestRunRespectsConcurrencyLimit(t *testing.T) {
	var current, peak atomic.Int32

	Run(context.Background(), missings(20), 3, func(ctx context.Context, m analyze.Missing) (string, error) {
		now := current.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		current.Add(-1)
		return "x", nil
	}, nil)

	if peak.Load() > 3 {
		t.Errorf("peak concurrency = %d, want <= 3", peak.Load())
	}
}

func TestRunCollectsErrors(t *testing.T) {
	boom := errors.New("boom")
	results := Run(context.Background(), missings(3), 2, func(ctx context.Context, m analyze.Missing) (string, error) {
		if m.Key == "key01" {
			return "", boom
		}
		return "ok", nil
	}, nil)

	if results[1].Err == nil || results[0].Err != nil || results[2].Err != nil {
		t.Errorf("errors misplaced: %+v", results)
	}
}

func TestRunReportsProgress(t *testing.T) {
	var mu sync.Mutex
	var seen []string

	Run(context.Background(), missings(5), 2, func(ctx context.Context, m analyze.Missing) (string, error) {
		return "x", nil
	}, func(r Result) {
		mu.Lock()
		seen = append(seen, r.Missing.Key)
		mu.Unlock()
	})

	if len(seen) != 5 {
		t.Errorf("progress callback fired %d times, want 5", len(seen))
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var started atomic.Int32
	results := Run(ctx, missings(50), 1, func(ctx context.Context, m analyze.Missing) (string, error) {
		if started.Add(1) == 3 {
			cancel()
		}
		return "x", nil
	}, nil)

	if n := started.Load(); n >= 50 {
		t.Errorf("cancellation ignored, %d translations started", n)
	}
	// Unprocessed items must carry the context error.
	last := results[len(results)-1]
	if last.Err == nil {
		t.Error("expected context error on unprocessed items")
	}
}
