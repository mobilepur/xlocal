// Package analyze finds missing translations in Xcode String Catalogs and
// collects the context needed to translate them.
package analyze

import (
	"sort"
	"strings"

	"github.com/MobilePur/xlocal/internal/xcstrings"
)

// Missing describes one untranslated (key, language) pair.
type Missing struct {
	Key            string
	SourceText     string
	TargetLanguage string
	FilePath       string
	Comment        string
	IsPlural       bool
	// Existing holds the actual translations in other languages, used as
	// context for the translation prompt.
	Existing map[string]string
}

// Report holds the analysis result for one catalog file.
type Report struct {
	FilePath        string
	TargetLanguages []string
	TotalStrings    int
	Missing         []Missing
}

// File analyzes a catalog for missing translations in targetLanguages.
// Results are ordered deterministically: keys in Xcode catalog order,
// languages in targetLanguages order.
//
// Keys marked "Don't Translate" in Xcode (shouldTranslate: false), keys
// listed in excludeKeys, and keys with no translatable source text are
// skipped.
func File(path string, catalog *xcstrings.File, targetLanguages []string, excludeKeys []string) *Report {
	report := &Report{
		FilePath:        path,
		TargetLanguages: targetLanguages,
		TotalStrings:    len(catalog.Strings),
	}

	keys := make([]string, 0, len(catalog.Strings))
	for key := range catalog.Strings {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j]) ||
			(strings.ToLower(keys[i]) == strings.ToLower(keys[j]) && keys[i] < keys[j])
	})

	excluded := make(map[string]bool, len(excludeKeys))
	for _, key := range excludeKeys {
		excluded[key] = true
	}

	for _, key := range keys {
		entry := catalog.Strings[key]

		if excluded[key] {
			continue
		}
		if entry.ShouldTranslate != nil && !*entry.ShouldTranslate {
			continue
		}

		sourceValue, hasSourceTranslation := localizedValue(entry.Localizations, catalog.SourceLanguage)

		sourceText := key
		if hasSourceTranslation {
			sourceText = sourceValue
		} else if entry.Comment != "" {
			sourceText = entry.Comment
		}

		// Nothing to translate: blank key without a source value or comment
		// (e.g. an empty key that slipped into the catalog).
		if strings.TrimSpace(sourceText) == "" {
			continue
		}

		isPlural := false
		if loc, ok := entry.Localizations[catalog.SourceLanguage]; ok {
			isPlural = loc.Variations != nil && loc.Variations.Plural != nil
		}

		for _, lang := range targetLanguages {
			if hasCompleteTranslation(entry.Localizations, lang) {
				continue
			}

			existing := make(map[string]string)
			for contextLang, loc := range entry.Localizations {
				if value, ok := entryValue(loc); ok {
					existing[contextLang] = value
				}
			}
			// Drop the source language when the key was never actually
			// translated there, so fallbacks don't pose as translations.
			if !hasSourceTranslation {
				delete(existing, catalog.SourceLanguage)
			}

			report.Missing = append(report.Missing, Missing{
				Key:            key,
				SourceText:     sourceText,
				TargetLanguage: lang,
				FilePath:       path,
				Comment:        entry.Comment,
				IsPlural:       isPlural,
				Existing:       existing,
			})
		}
	}

	return report
}

// localizedValue returns the representative value of a localization: the
// string unit value, or the "other" case for plural entries.
func localizedValue(localizations map[string]xcstrings.LocalizationEntry, lang string) (string, bool) {
	loc, ok := localizations[lang]
	if !ok {
		return "", false
	}
	return entryValue(loc)
}

func entryValue(loc xcstrings.LocalizationEntry) (string, bool) {
	if loc.StringUnit != nil && strings.TrimSpace(loc.StringUnit.Value) != "" {
		return loc.StringUnit.Value, true
	}
	if loc.Variations != nil && loc.Variations.Plural != nil {
		if other, ok := loc.Variations.Plural["other"]; ok && strings.TrimSpace(other.StringUnit.Value) != "" {
			return other.StringUnit.Value, true
		}
	}
	return "", false
}

// hasCompleteTranslation reports whether lang is fully translated: a
// non-empty string unit, or for plurals both "one" and "other" forms.
func hasCompleteTranslation(localizations map[string]xcstrings.LocalizationEntry, lang string) bool {
	loc, ok := localizations[lang]
	if !ok {
		return false
	}
	if loc.StringUnit != nil && strings.TrimSpace(loc.StringUnit.Value) != "" {
		return true
	}
	if loc.Variations != nil && loc.Variations.Plural != nil {
		one, hasOne := loc.Variations.Plural["one"]
		other, hasOther := loc.Variations.Plural["other"]
		return hasOne && strings.TrimSpace(one.StringUnit.Value) != "" &&
			hasOther && strings.TrimSpace(other.StringUnit.Value) != ""
	}
	return false
}

// CheckUntranslatableWords returns the words that appear in the source text
// but are missing from the translation (case-insensitive) — brand names and
// product terms that must never be translated.
func CheckUntranslatableWords(source, translation string, untranslatableWords []string) []string {
	var violated []string

	sourceLower := strings.ToLower(source)
	translationLower := strings.ToLower(translation)

	for _, word := range untranslatableWords {
		wordLower := strings.ToLower(word)
		if strings.Contains(sourceLower, wordLower) && !strings.Contains(translationLower, wordLower) {
			violated = append(violated, word)
		}
	}

	return violated
}
