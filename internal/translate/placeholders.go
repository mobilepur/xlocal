package translate

import (
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/xcstrings"
)

// Recognize Foundation's printf arguments, including positional arguments,
// length modifiers, flags, width and precision. Escaped percent signs must
// be consumed first so that %%d is never mistaken for an argument.
var formatSpecifier = regexp.MustCompile(`%%|%([0-9]+\$)?[-+ #0']*([0-9]+|\*([0-9]+\$)?)?(\.(\*([0-9]+\$)?|[0-9]*))?(hh|ll|[hlqLztj])?[A-Za-z@]`)
var argumentPosition = regexp.MustCompile(`[0-9]+\$`)

// ValidatePluralTranslation verifies categories and format arguments before
// a response can be saved. Every target form must use a source form's argument
// signature. This permits source-backed number omissions without permitting
// invented types, changed argument positions or dropped unrelated arguments.
// Whether an omission is grammatical for the target category remains part of
// the translation prompt and user review.
func ValidatePluralTranslation(response string, m analyze.Missing) error {
	if err := ValidatePluralForms(response, m.TargetLanguage); err != nil {
		return err
	}

	sources := []string{m.SourceText}
	if len(m.SourcePluralForms) > 0 {
		sources = nil
		categories := make([]string, 0, len(m.SourcePluralForms))
		for category := range m.SourcePluralForms {
			categories = append(categories, category)
		}
		sort.Strings(categories)
		for _, category := range categories {
			if value := m.SourcePluralForms[category]; strings.TrimSpace(value) != "" {
				sources = append(sources, value)
			}
		}
	}

	expected := make([]map[formatArgument]int, 0, len(sources))
	for _, source := range sources {
		args, err := formatArguments(source)
		if err != nil {
			return fmt.Errorf("source plural format: %w", err)
		}
		expected = append(expected, args)
	}

	categories := xcstrings.PluralCategories(m.TargetLanguage)
	forms := ParsePluralForms(response, categories)
	for _, category := range categories {
		args, err := formatArguments(forms[category])
		if err != nil {
			return fmt.Errorf("plural %s format: %w", category, err)
		}
		matches := false
		for _, sourceArgs := range expected {
			if maps.Equal(args, sourceArgs) {
				matches = true
				break
			}
		}
		if !matches {
			return fmt.Errorf("plural %s changes the source format arguments: %q", category, forms[category])
		}
	}
	return nil
}

// Each use retains its formatting and dynamic width/precision bindings.
// Counts preserve repeated uses; types are checked separately per argument.
type formatArgument struct {
	position  int
	specifier string
	width     int
	precision int
}

func formatArguments(text string) (map[formatArgument]int, error) {
	args := make(map[formatArgument]int)
	types := make(map[int]string)
	next := 1
	explicit, implicit := false, false
	add := func(position, valueType string) (int, error) {
		index := next
		if position != "" {
			explicit = true
			var err error
			index, err = strconv.Atoi(strings.TrimSuffix(position, "$"))
			if err != nil || index < 1 {
				return 0, fmt.Errorf("invalid argument position %q", position)
			}
		} else {
			implicit = true
			next++
		}
		if previous, ok := types[index]; ok && previous != valueType {
			return 0, fmt.Errorf("argument %d uses incompatible types %s and %s", index, previous, valueType)
		}
		types[index] = valueType
		return index, nil
	}

	for _, match := range formatSpecifier.FindAllStringSubmatch(text, -1) {
		if match[0] == "%%" {
			continue
		}
		arg := formatArgument{specifier: argumentPosition.ReplaceAllString(match[0], "")}
		var err error
		// Dynamic width and precision consume integer arguments before the
		// value when the format uses sequential arguments.
		if strings.HasPrefix(match[2], "*") {
			arg.width, err = add(match[3], "d")
			if err != nil {
				return nil, err
			}
		}
		if strings.HasPrefix(match[5], "*") {
			arg.precision, err = add(match[6], "d")
			if err != nil {
				return nil, err
			}
		}
		conversion := match[0][len(match[0])-1:]
		if !strings.Contains("@dDiuUoOxXfFeEgGaAcCsSp", conversion) {
			return nil, fmt.Errorf("unsupported format specifier %q", match[0])
		}
		// Conversions can share a runtime type while formatting it differently.
		// Keep exact formatting in args; normalize only the compatibility check.
		family := conversion
		switch {
		case strings.Contains("dDi", conversion):
			family = "d"
		case strings.Contains("uUoOxX", conversion):
			family = "u"
		case strings.Contains("fFeEgGaA", conversion):
			family = "f"
		}
		valueType := match[7] + family
		arg.position, err = add(match[1], valueType)
		if err != nil {
			return nil, err
		}
		args[arg]++
	}
	if explicit && implicit {
		return nil, fmt.Errorf("cannot mix positional and sequential format arguments")
	}
	return args, nil
}
