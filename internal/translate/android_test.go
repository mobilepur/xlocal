package translate

import (
	"github.com/MobilePur/xlocal/internal/analyze"
	"strings"
	"testing"
)

func TestAndroidPluralAndUnformattedValidation(t *testing.T) {
	m := analyze.Missing{Platform: analyze.PlatformAndroid, SourceText: "%1$d files", SourceLanguage: "en", TargetLanguage: "cs", IsPlural: true, SourcePluralForms: map[string]string{"one": "%1$d file", "other": "%1$d files"}}
	valid := "one: %1$d soubor | few: %1$d soubory | other: %1$d souborů"
	if err := ValidateTranslation(valid, m); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"one: %1$d soubor | other: %1$d souborů", "one: %1$s soubor | few: %1$d soubory | other: %1$d souborů", valid + " | other: %1$d duplicate", valid + " | unknown: %1$d"} {
		if err := ValidateTranslation(bad, m); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	prompt := BuildPrompt(m, Options{})
	if strings.Contains(prompt, "many:") || strings.Contains(prompt, "iOS") || !strings.Contains(prompt, "Android") {
		t.Fatal(prompt)
	}
	m.IsPlural = false
	m.SourceText = "100% ready"
	m.AndroidUnformatted = true
	if err := ValidateTranslation("100% připraveno", m); err != nil {
		t.Fatal(err)
	}
}
