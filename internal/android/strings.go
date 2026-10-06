package android

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// AAPT2 collapses XML's ASCII whitespace, preserving Unicode spaces such as
// NBSP and thin space. Quoted or escaped whitespace remains literal.
func isAndroidWhitespace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }

// decodeString follows Android quote/escape and whitespace rules for plain text.
func decodeString(s string) (string, error) {
	var out strings.Builder
	quoted := false
	pendingSpace := false
	started := false
	appendRune := func(r rune, preserve bool) {
		if !preserve && isAndroidWhitespace(r) {
			if started {
				pendingSpace = true
			}
			return
		}
		if pendingSpace {
			out.WriteByte(' ')
			pendingSpace = false
		}
		out.WriteRune(r)
		started = true
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return "", errors.New("invalid UTF-8 resource")
		}
		i += size
		if r == '"' {
			quoted = !quoted
			continue
		}
		if r == '\\' {
			if i >= len(s) {
				return "", errors.New("trailing Android escape")
			}
			r, size = utf8.DecodeRuneInString(s[i:])
			i += size
			switch r {
			case 'n':
				r = '\n'
			case 't':
				r = '\t'
			case 'r':
				r = 'r'
			case '\\', '\'', '"', '@', '?':
			case 'u':
				if i+4 > len(s) {
					return "", errors.New("incomplete Unicode escape")
				}
				v, e := strconv.ParseUint(s[i:i+4], 16, 16)
				if e != nil {
					return "", errors.New("invalid Unicode escape")
				}
				i += 4
				r = rune(v)
				if r >= 0xD800 && r <= 0xDFFF {
					return "", errors.New("UTF-16 surrogate escapes are unsupported by Android resources; use literal Unicode")
				}
			default:
				return "", fmt.Errorf("unsupported Android escape \\%c", r)
			}
			appendRune(r, true)
			continue
		}
		if r == '\'' && !quoted {
			return "", errors.New("unescaped Android apostrophe")
		}
		appendRune(r, quoted)
	}
	if quoted {
		return "", errors.New("unclosed Android string quote")
	}
	return out.String(), nil
}
func encodeString(text string) (string, error) {
	if !utf8.ValidString(text) {
		return "", errors.New("translation contains invalid UTF-8")
	}
	var android strings.Builder
	android.WriteByte('"')
	for _, r := range text {
		if !(r == 9 || r == 10 || r == 13 || r >= 32 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF) {
			return "", fmt.Errorf("translation contains invalid XML character U+%04X", r)
		}
		switch r {
		case '\\':
			android.WriteString(`\\`)
		case '"':
			android.WriteString(`\"`)
		case '\'':
			android.WriteString(`\'`)
		case '\n':
			android.WriteString(`\n`)
		case '\t':
			android.WriteString(`\t`)
		case '\r':
			android.WriteString(`\u000d`)
		default:
			android.WriteRune(r)
		}
	}
	android.WriteByte('"')
	var result bytes.Buffer
	if e := xml.EscapeText(&result, []byte(android.String())); e != nil {
		return "", e
	}
	return result.String(), nil
}
