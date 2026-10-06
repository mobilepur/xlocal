package analyze

import (
	"fmt"
	"slices"
	"strings"

	"github.com/MobilePur/xlocal/internal/android"
)

// AndroidFile compares explicit source values with localized resources in the
// same resource root. A resource name is an identifier, never fallback text.
func AndroidFile(path, sourceLanguage string, catalog *android.Catalog, targetLanguages, excludeKeys []string) (*Report, error) {
	normalizedSource, err := android.NormalizeLanguage(sourceLanguage)
	if err != nil {
		return nil, err
	}
	sourceLanguage = normalizedSource
	normalizedTargets := make([]string, 0, len(targetLanguages))
	for _, lang := range targetLanguages {
		normalized, err := android.NormalizeLanguage(lang)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(normalizedTargets, normalized) {
			normalizedTargets = append(normalizedTargets, normalized)
		}
	}
	targetLanguages = normalizedTargets
	report := &Report{FilePath: path, TargetLanguages: targetLanguages, Warnings: catalog.Warnings}
	for _, lang := range targetLanguages {
		if _, err := android.LocaleFolder(lang); err != nil {
			return nil, err
		}
	}
	for _, entry := range catalog.Entries {
		if !entry.Translatable || entry.SkipReason != "" || slices.Contains(excludeKeys, entry.Key) {
			continue
		}
		plural := entry.PluralForms != nil
		text := entry.Text
		if plural {
			text = entry.PluralForms["other"]
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		if entry.Formatted {
			texts := []string{text}
			if plural {
				texts = nil
				for _, value := range entry.PluralForms {
					texts = append(texts, value)
				}
			}
			for _, value := range texts {
				if err := android.ValidateFormat(value, value); err != nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("%s %s: unsupported source format (%v); skipped. Use formatted=false for literal percent text", path, entry.Key, err))
					text = ""
					break
				}
			}
		}
		if text == "" {
			continue
		}
		report.TotalStrings++
		id := android.ResourceID(entry.Key, plural)
		for _, lang := range targetLanguages {
			if strings.EqualFold(lang, sourceLanguage) {
				continue
			}
			target, exists := catalog.Localizations[lang][id]
			complete := exists && target.SkipReason == ""
			if exists && target.SkipReason != "" {
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s %s: existing %s translation is unsupported (%s); skipped", path, entry.Key, lang, target.SkipReason))
				continue
			}
			if plural {
				categories, err := android.PluralCategories(lang)
				if err != nil {
					return nil, fmt.Errorf("%s %s: %w", path, entry.Key, err)
				}
				for _, category := range categories {
					if strings.TrimSpace(target.PluralForms[category]) == "" {
						complete = false
					}
				}
			} else {
				complete = complete && strings.TrimSpace(target.Text) != ""
			}
			if complete {
				continue
			}
			if exists && (!target.Translatable || target.Formatted != entry.Formatted) {
				reason := "formatted attribute differs from source"
				if !target.Translatable {
					reason = "target has translatable=false"
				}
				report.Warnings = append(report.Warnings, fmt.Sprintf("%s %s: existing %s translation cannot be filled safely (%s); skipped", path, entry.Key, lang, reason))
				continue
			}
			existing := map[string]string{sourceLanguage: text}
			for contextLang, entries := range catalog.Localizations {
				context, ok := entries[id]
				if !ok || context.SkipReason != "" {
					continue
				}
				value := context.Text
				if plural {
					value = context.PluralForms["other"]
				}
				if strings.TrimSpace(value) != "" {
					existing[contextLang] = value
				}
			}
			report.Missing = append(report.Missing, Missing{Key: entry.Key, Platform: PlatformAndroid, SourceLanguage: sourceLanguage, AndroidUnformatted: !entry.Formatted, SourceText: text, TargetLanguage: lang, FilePath: path, Comment: entry.Comment, IsPlural: plural, SourcePluralForms: entry.PluralForms, Existing: existing})
		}
	}
	return report, nil
}
