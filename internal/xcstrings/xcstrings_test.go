package xcstrings

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The golden fixtures in testdata/ are written in the exact format Xcode's
// String Catalog editor produces. Loading and re-marshaling them must be a
// byte-identical round trip, otherwise xlocal would create noisy diffs in
// files that are also edited by Xcode.
func TestRoundTripPreservesXcodeFormat(t *testing.T) {
	for _, name := range []string{"simple.xcstrings", "plural.xcstrings"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			file, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			out, err := Marshal(file)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			if !bytes.Equal(original, out) {
				t.Errorf("round trip is not byte-identical\n--- original ---\n%s\n--- marshaled ---\n%s", original, out)
			}
		})
	}
}

func TestLoadReadsEntries(t *testing.T) {
	file, err := Load(filepath.Join("testdata", "simple.xcstrings"))
	if err != nil {
		t.Fatal(err)
	}

	if file.SourceLanguage != "en" {
		t.Errorf("SourceLanguage = %q, want %q", file.SourceLanguage, "en")
	}
	if file.Version != "1.0" {
		t.Errorf("Version = %q, want %q", file.Version, "1.0")
	}
	if len(file.Strings) != 5 {
		t.Errorf("len(Strings) = %d, want 5", len(file.Strings))
	}

	entry, ok := file.Strings["cancel"]
	if !ok {
		t.Fatal(`missing key "cancel"`)
	}
	if entry.Comment != "Cancel" {
		t.Errorf("Comment = %q, want %q", entry.Comment, "Cancel")
	}
	if got := entry.Localizations["de"].StringUnit.Value; got != "Abbrechen" {
		t.Errorf(`de value = %q, want "Abbrechen"`, got)
	}
}

func TestLoadReadsPluralVariations(t *testing.T) {
	file, err := Load(filepath.Join("testdata", "plural.xcstrings"))
	if err != nil {
		t.Fatal(err)
	}

	loc := file.Strings["documentPages"].Localizations["de"]
	if loc.StringUnit != nil {
		t.Error("plural entry must not have a StringUnit")
	}
	if loc.Variations == nil || loc.Variations.Plural == nil {
		t.Fatal("plural variations missing")
	}
	if got := loc.Variations.Plural["other"].StringUnit.Value; got != "%lld Seiten" {
		t.Errorf(`plural other = %q, want "%%lld Seiten"`, got)
	}
}

func TestSetTranslation(t *testing.T) {
	file, err := Load(filepath.Join("testdata", "simple.xcstrings"))
	if err != nil {
		t.Fatal(err)
	}

	if err := file.SetTranslation("welcome %@", "fr", "Bienvenue %@!"); err != nil {
		t.Fatalf("SetTranslation: %v", err)
	}

	loc := file.Strings["welcome %@"].Localizations["fr"]
	if loc.StringUnit == nil || loc.StringUnit.Value != "Bienvenue %@!" {
		t.Fatalf("fr translation not set: %+v", loc)
	}
	if loc.StringUnit.State != "translated" {
		t.Errorf("State = %q, want \"translated\"", loc.StringUnit.State)
	}
}

func TestSetTranslationUnknownKey(t *testing.T) {
	file, err := Load(filepath.Join("testdata", "simple.xcstrings"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.SetTranslation("nope", "fr", "non"); err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestSetPluralTranslation(t *testing.T) {
	file, err := Load(filepath.Join("testdata", "plural.xcstrings"))
	if err != nil {
		t.Fatal(err)
	}

	if err := file.SetPluralTranslation("documentPages", "fr", "1 page", "%lld pages"); err != nil {
		t.Fatalf("SetPluralTranslation: %v", err)
	}

	loc := file.Strings["documentPages"].Localizations["fr"]
	if loc.Variations == nil || loc.Variations.Plural == nil {
		t.Fatal("plural variations not set")
	}
	if got := loc.Variations.Plural["one"].StringUnit.Value; got != "1 page" {
		t.Errorf(`one = %q, want "1 page"`, got)
	}
	if got := loc.Variations.Plural["other"].StringUnit.Value; got != "%lld pages" {
		t.Errorf(`other = %q, want "%%lld pages"`, got)
	}
	if got := loc.Variations.Plural["other"].StringUnit.State; got != "translated" {
		t.Errorf(`state = %q, want "translated"`, got)
	}
}

// After a mutation the output must still follow Xcode's ordering rules:
// case-insensitive sort, lowercase variant first on a case-only tie.
func TestMarshalOrderingAfterMutation(t *testing.T) {
	file, err := Load(filepath.Join("testdata", "simple.xcstrings"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.SetTranslation("create", "de", "erstellen"); err != nil {
		t.Fatal(err)
	}

	out, err := Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	lower := strings.Index(s, "\"create\" :")
	upper := strings.Index(s, "\"Create\" :")
	cancel := strings.Index(s, "\"cancel\" :")
	if lower == -1 || upper == -1 || cancel == -1 {
		t.Fatalf("expected keys not found in output:\n%s", s)
	}
	if !(cancel < lower && lower < upper) {
		t.Errorf("key order wrong: cancel=%d create=%d Create=%d", cancel, lower, upper)
	}
}

func TestMarshalDoesNotEscapeHTML(t *testing.T) {
	file, err := Load(filepath.Join("testdata", "simple.xcstrings"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	// If & or < were HTML-escaped as unicode sequences, the literal
	// substring would not survive in the output.
	if !strings.Contains(string(out), "Terms & Conditions <legal>") {
		t.Error("special characters were escaped, Xcode writes them literally")
	}
}

func TestSaveWritesFile(t *testing.T) {
	file, err := Load(filepath.Join("testdata", "simple.xcstrings"))
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "out.xcstrings")
	if err := file.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if len(reloaded.Strings) != len(file.Strings) {
		t.Errorf("reloaded %d strings, want %d", len(reloaded.Strings), len(file.Strings))
	}
}
