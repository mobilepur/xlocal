// Package xcstrings reads and writes Xcode String Catalog (.xcstrings) files.
//
// Marshal reproduces the exact output format of Xcode's String Catalog
// editor (two-space indentation, " : " key separator, case-insensitive key
// ordering, no HTML escaping, no trailing newline) so that files edited by
// both xlocal and Xcode produce minimal diffs.
package xcstrings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type File struct {
	SourceLanguage string                 `json:"sourceLanguage"`
	Strings        map[string]StringEntry `json:"strings"`
	Version        string                 `json:"version"`
}

type StringEntry struct {
	Comment         string                       `json:"comment,omitempty"`
	ExtractionState string                       `json:"extractionState,omitempty"`
	Localizations   map[string]LocalizationEntry `json:"localizations,omitempty"`
	// ShouldTranslate is Xcode's "Don't Translate" flag; a pointer so that an
	// explicit false survives the load/save round trip.
	ShouldTranslate *bool `json:"shouldTranslate,omitempty"`
}

type LocalizationEntry struct {
	StringUnit *StringUnit       `json:"stringUnit,omitempty"`
	Variations *PluralVariations `json:"variations,omitempty"`
}

type PluralVariations struct {
	Plural map[string]PluralCase `json:"plural"`
}

type PluralCase struct {
	StringUnit StringUnit `json:"stringUnit"`
}

type StringUnit struct {
	State string `json:"state,omitempty"`
	Value string `json:"value"`
}

const StateTranslated = "translated"

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: invalid string catalog: %w", path, err)
	}

	return &file, nil
}

func (f *File) Save(path string) error {
	data, err := Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SetTranslation sets a regular (non-plural) translation for key in lang.
func (f *File) SetTranslation(key, lang, value string) error {
	entry, ok := f.Strings[key]
	if !ok {
		return fmt.Errorf("key %q not found in catalog", key)
	}

	if entry.Localizations == nil {
		entry.Localizations = make(map[string]LocalizationEntry)
	}
	entry.Localizations[lang] = LocalizationEntry{
		StringUnit: &StringUnit{State: StateTranslated, Value: value},
	}
	f.Strings[key] = entry
	return nil
}

// SetPluralTranslation sets a plural translation with "one" and "other"
// forms for key in lang.
func (f *File) SetPluralTranslation(key, lang, one, other string) error {
	entry, ok := f.Strings[key]
	if !ok {
		return fmt.Errorf("key %q not found in catalog", key)
	}

	if entry.Localizations == nil {
		entry.Localizations = make(map[string]LocalizationEntry)
	}
	entry.Localizations[lang] = LocalizationEntry{
		Variations: &PluralVariations{
			Plural: map[string]PluralCase{
				"one":   {StringUnit: StringUnit{State: StateTranslated, Value: one}},
				"other": {StringUnit: StringUnit{State: StateTranslated, Value: other}},
			},
		},
	}
	f.Strings[key] = entry
	return nil
}

// Marshal serializes the catalog in Xcode's exact String Catalog format.
func Marshal(f *File) ([]byte, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}

	var generic interface{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	encodeXcodeValue(&buf, generic, "")
	return buf.Bytes(), nil
}

func encodeXcodeValue(buf *bytes.Buffer, v interface{}, indent string) {
	switch val := v.(type) {
	case map[string]interface{}:
		if len(val) == 0 {
			buf.WriteString("{}")
			return
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.SliceStable(keys, func(i, j int) bool {
			return xcodeKeyLess(keys[i], keys[j])
		})
		buf.WriteString("{\n")
		inner := indent + "  "
		for i, k := range keys {
			buf.WriteString(inner)
			encodeXcodeString(buf, k)
			buf.WriteString(" : ")
			encodeXcodeValue(buf, val[k], inner)
			if i < len(keys)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(indent + "}")
	case []interface{}:
		if len(val) == 0 {
			buf.WriteString("[]")
			return
		}
		buf.WriteString("[\n")
		inner := indent + "  "
		for i, item := range val {
			buf.WriteString(inner)
			encodeXcodeValue(buf, item, inner)
			if i < len(val)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteString(indent + "]")
	case string:
		encodeXcodeString(buf, val)
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case json.Number:
		buf.WriteString(val.String())
	case nil:
		buf.WriteString("null")
	}
}

// xcodeKeyLess orders keys the way Xcode's String Catalog does: primarily
// case-insensitive, and for keys that differ only in case the lowercase
// variant comes first (e.g. "create" before "Create").
func xcodeKeyLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return len(a) < len(b)
}

func encodeXcodeString(buf *bytes.Buffer, s string) {
	var tmp bytes.Buffer
	enc := json.NewEncoder(&tmp)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	buf.Write(bytes.TrimRight(tmp.Bytes(), "\n"))
}
