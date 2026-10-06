package android

import (
	"fmt"
	"strings"

	"golang.org/x/text/language"
)

// PluralCategories returns the cardinal categories reachable by integer
// quantities, plus Android's mandatory "other" fallback. It deliberately
// rejects unverified languages instead of assuming English rules.
// The supported table is derived from CLDR 48's cardinal rules (v=f=0 for
// integer quantities). Fractional-only many in cs/sk/lt is excluded; many
// at integer multiples of 1,000,000 in fr/es/it/pt is included. Android OS
// versions bundle different CLDR revisions; extra forms are harmless and
// "other" is retained even when no integer quantity selects it (ru/uk/pl).
// https://raw.githubusercontent.com/unicode-org/cldr/release-48/common/supplemental/plurals.xml
// https://developer.android.com/guide/topics/resources/string-resource#Plurals
func PluralCategories(lang string) ([]string, error) {
	tag, err := language.Parse(strings.TrimSpace(lang))
	if err != nil {
		return nil, fmt.Errorf("invalid Android plural language %q: %w", lang, err)
	}
	base, _, _ := tag.Raw()
	var categories []string
	switch base.String() {
	case "ja", "ko", "zh", "id", "th", "vi", "ms":
		categories = []string{"other"}
	case "en", "de", "nl", "sv", "da", "fi", "no", "nb", "nn", "tr", "el", "hu", "bg", "et", "hi", "fa":
		categories = []string{"one", "other"}
	case "fr", "es", "it", "pt", "ca":
		categories = []string{"one", "many", "other"}
	case "cs", "sk", "lt", "ro", "sr", "hr", "bs":
		categories = []string{"one", "few", "other"}
	case "ru", "uk", "pl", "be":
		categories = []string{"one", "few", "many", "other"}
	case "he":
		categories = []string{"one", "two", "other"}
	case "sl":
		categories = []string{"one", "two", "few", "other"}
	case "lv":
		categories = []string{"zero", "one", "other"}
	case "ar":
		categories = []string{"zero", "one", "two", "few", "many", "other"}
	default:
		return nil, fmt.Errorf("Android integer plural categories for language %q are not supported by the local prototype", lang)
	}
	return categories, nil
}
