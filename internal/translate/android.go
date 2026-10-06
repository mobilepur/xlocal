package translate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/android"
	"github.com/MobilePur/xlocal/internal/xcstrings"
)

// PluralCategoriesFor keeps platform-specific quantity rules at the format
// boundary. Android quantities are integers; Foundation also supports fractions.
func PluralCategoriesFor(m analyze.Missing) ([]string, error) {
	if m.Platform == analyze.PlatformAndroid {
		return android.PluralCategories(m.TargetLanguage)
	}
	return xcstrings.PluralCategories(m.TargetLanguage), nil
}

// ValidateTranslation protects both plain and plural Android format arguments.
// Xcode retains its existing validation behavior.
func ValidateTranslation(response string, m analyze.Missing) error {
	if m.Platform != analyze.PlatformAndroid {
		if m.IsPlural {
			return ValidatePluralTranslation(response, m)
		}
		return nil
	}
	if !m.IsPlural {
		if m.AndroidUnformatted {
			return nil
		}
		return android.ValidateFormat(m.SourceText, response)
	}
	categories, err := PluralCategoriesFor(m)
	if err != nil {
		return err
	}
	forms, err := parseAndroidPluralForms(response, categories)
	if err != nil {
		return err
	}
	if m.AndroidUnformatted {
		return nil
	}
	sources := []string{m.SourceText}
	if len(m.SourcePluralForms) > 0 {
		sources = nil
		for _, value := range m.SourcePluralForms {
			if strings.TrimSpace(value) != "" {
				sources = append(sources, value)
			}
		}
	}
	for _, category := range categories {
		matches := false
		for _, source := range sources {
			if android.ValidateFormat(source, forms[category]) == nil {
				matches = true
				break
			}
		}
		if !matches {
			return fmt.Errorf("Android plural %s changes the source format arguments", category)
		}
	}
	return nil
}

func parseAndroidPluralForms(response string, categories []string) (map[string]string, error) {
	if len(categories) == 1 && !strings.Contains(response, ": ") {
		if strings.TrimSpace(response) == "" {
			return nil, fmt.Errorf("empty plural translation")
		}
		return map[string]string{categories[0]: response}, nil
	}
	forms := map[string]string{}
	for _, part := range strings.Split(response, " | ") {
		category, value, ok := strings.Cut(strings.TrimSpace(part), ": ")
		if !ok || !slices.Contains(categories, category) || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("invalid Android plural form %q", part)
		}
		if _, exists := forms[category]; exists {
			return nil, fmt.Errorf("duplicate Android plural form %s", category)
		}
		forms[category] = strings.TrimSpace(value)
	}
	for _, category := range categories {
		if forms[category] == "" {
			return nil, fmt.Errorf("Android plural response is missing %s", category)
		}
	}
	return forms, nil
}
