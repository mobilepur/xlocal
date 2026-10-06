package android

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
)

// ValidateFormat preserves every Java Formatter use, including its argument
// index, conversion, flags, width and precision. Equivalent explicit indexing
// and relative indexing are allowed, so translations can reorder arguments.
// Literal percent/newline conversions consume no arguments, but are preserved.
// Callers must bypass this check for resources with formatted="false".
// Grammar: https://developer.android.com/reference/java/util/Formatter
func ValidateFormat(source, target string) error {
	expected, err := javaFormats(source)
	if err != nil {
		return fmt.Errorf("source Java format: %w", err)
	}
	actual, err := javaFormats(target)
	if err != nil {
		return fmt.Errorf("translation Java format: %w", err)
	}
	if !maps.Equal(expected, actual) {
		return fmt.Errorf("translation changes Java format arguments or formatting")
	}
	return nil
}

type javaFormat struct {
	position   int
	flags      string
	width      int
	precision  int
	conversion string
}

func javaFormats(text string) (map[javaFormat]int, error) {
	uses := make(map[javaFormat]int)
	next, previous := 1, 0
	for i := 0; i < len(text); {
		if text[i] != '%' {
			i++
			continue
		}
		start := i
		i++
		spec := javaFormat{precision: -1}
		// An initial digit sequence is an explicit argument only when followed by $.
		digits := i
		for i < len(text) && asciiDigit(text[i]) {
			i++
		}
		explicit := 0
		if i < len(text) && i > digits && text[i] == '$' {
			var err error
			explicit, err = javaNumber(text[digits:i], false)
			if err != nil {
				return nil, fmt.Errorf("at byte %d: argument index: %w", start, err)
			}
			i++
		} else {
			i = digits
		}

		flags := make(map[byte]bool)
		for i < len(text) && strings.ContainsRune("-#+ 0,(<", rune(text[i])) {
			flag := text[i]
			if flags[flag] {
				return nil, fmt.Errorf("at byte %d: duplicate flag %q", start, flag)
			}
			flags[flag] = true
			i++
		}
		digits = i
		for i < len(text) && asciiDigit(text[i]) {
			i++
		}
		if i > digits {
			var err error
			spec.width, err = javaNumber(text[digits:i], false)
			if err != nil {
				return nil, fmt.Errorf("at byte %d: width: %w", start, err)
			}
		}
		if i < len(text) && text[i] == '.' {
			i++
			digits = i
			for i < len(text) && asciiDigit(text[i]) {
				i++
			}
			var err error
			spec.precision, err = javaNumber(text[digits:i], true)
			if err != nil {
				return nil, fmt.Errorf("at byte %d: precision: %w", start, err)
			}
		}
		if i >= len(text) {
			return nil, fmt.Errorf("at byte %d: incomplete format specifier", start)
		}
		conversion := text[i]
		i++
		spec.conversion = string(conversion)
		date := conversion == 't' || conversion == 'T'
		if date {
			if i >= len(text) || !strings.ContainsRune("HIklMSLNpzZsQBbhAaCYyjmdeRTrDFc", rune(text[i])) {
				return nil, fmt.Errorf("at byte %d: invalid date/time conversion", start)
			}
			spec.conversion += string(text[i])
			i++
		} else if !strings.ContainsRune("bBhHsScCdoxXeEfgGaA%n", rune(conversion)) {
			return nil, fmt.Errorf("at byte %d: unsupported Java conversion %q", start, conversion)
		}

		allowed := "-"
		precisionAllowed := false
		switch {
		case date:
		case strings.ContainsRune("bBhH", rune(conversion)):
			precisionAllowed = true
		case conversion == 's' || conversion == 'S':
			allowed = "-#"
			precisionAllowed = true
		case conversion == 'd':
			allowed = "-+ 0,("
		case strings.ContainsRune("oxX", rune(conversion)):
			allowed = "-#+ 0("
		case strings.ContainsRune("eE", rune(conversion)):
			allowed = "-#+ 0("
			precisionAllowed = true
		case conversion == 'f':
			allowed = "-#+ 0,("
			precisionAllowed = true
		case conversion == 'g' || conversion == 'G':
			allowed = "-+ 0,("
			precisionAllowed = true
		case conversion == 'a' || conversion == 'A':
			allowed = "-#+ 0"
			precisionAllowed = true
		case conversion == 'n':
			allowed = ""
		}
		argument := date || (conversion != '%' && conversion != 'n')
		for flag := range flags {
			if flag == '<' && argument {
				continue
			}
			if !strings.ContainsRune(allowed, rune(flag)) {
				return nil, fmt.Errorf("at byte %d: flag %q is invalid for %s", start, flag, spec.conversion)
			}
		}
		if spec.precision >= 0 && !precisionAllowed {
			return nil, fmt.Errorf("at byte %d: precision is invalid for %s", start, spec.conversion)
		}
		if conversion == 'n' && spec.width != 0 {
			return nil, fmt.Errorf("at byte %d: newline cannot have width", start)
		}
		if (flags['-'] || flags['0']) && spec.width == 0 {
			return nil, fmt.Errorf("at byte %d: flag requires width", start)
		}
		if flags['+'] && flags[' '] || flags['-'] && flags['0'] {
			return nil, fmt.Errorf("at byte %d: incompatible flags", start)
		}
		if argument {
			switch {
			case flags['<']:
				if previous == 0 {
					return nil, fmt.Errorf("at byte %d: relative argument has no previous argument", start)
				}
				spec.position = previous
			case explicit != 0:
				spec.position = explicit
			default:
				spec.position = next
				next++
			}
			previous = spec.position
		}
		// Argument indexes on non-argument conversions are ignored by Formatter.
		// Sort flags so their insignificant ordering cannot reject a translation.
		canonical := make([]byte, 0, len(flags))
		for flag := range flags {
			if flag != '<' {
				canonical = append(canonical, flag)
			}
		}
		sort.Slice(canonical, func(i, j int) bool { return canonical[i] < canonical[j] })
		spec.flags = string(canonical)
		uses[spec]++
	}
	return uses, nil
}

func asciiDigit(c byte) bool { return c >= '0' && c <= '9' }

func javaNumber(text string, zeroAllowed bool) (int, error) {
	n, err := strconv.ParseUint(text, 10, 31)
	if err != nil || n == 0 && !zeroAllowed {
		return 0, fmt.Errorf("invalid value %q (Java requires a 32-bit nonnegative value and a positive index/width)", text)
	}
	return int(n), nil
}
