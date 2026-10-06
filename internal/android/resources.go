// Package android reads Android value resources and applies translations without
// rewriting unrelated XML. Each resource directory is its own namespace.
package android

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Entry struct {
	Key, Text, Comment string
	PluralForms        map[string]string
	Formatted          bool
	Translatable       bool
	SkipReason         string
}
type Catalog struct {
	Entries       []Entry
	Localizations map[string]map[string]Entry
	Warnings      []string
}
type Translation struct {
	Key, Text   string
	PluralForms map[string]string
}

func ResourceID(key string, plural bool) string {
	if plural {
		return "plurals/" + key
	}
	return "string/" + key
}

var languageRE = regexp.MustCompile(`^[A-Za-z]{2,3}$`)
var scriptRE = regexp.MustCompile(`^[A-Za-z]{4}$`)
var regionRE = regexp.MustCompile(`^(?:[A-Za-z]{2}|[0-9]{3})$`)
var variantRE = regexp.MustCompile(`^(?:[A-Za-z0-9]{5,8}|[0-9][A-Za-z0-9]{3})$`)
var keyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var quantities = map[string]bool{"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true}
var quantityOrder = []string{"zero", "one", "two", "few", "many", "other"}

func normalizeLanguage(language string) (string, error) {
	p := strings.Split(language, "-")
	if len(p) == 0 || !languageRE.MatchString(p[0]) {
		return "", fmt.Errorf("invalid Android locale %q", language)
	}
	p[0] = strings.ToLower(p[0])
	switch p[0] {
	case "iw":
		p[0] = "he"
	case "in":
		p[0] = "id"
	case "ji":
		p[0] = "yi"
	}
	i := 1
	if i < len(p) && scriptRE.MatchString(p[i]) {
		p[i] = strings.ToUpper(p[i][:1]) + strings.ToLower(p[i][1:])
		i++
	}
	if i < len(p) && regionRE.MatchString(p[i]) {
		p[i] = strings.ToUpper(p[i])
		i++
	}
	for ; i < len(p); i++ {
		if !variantRE.MatchString(p[i]) {
			return "", fmt.Errorf("unsupported Android locale %q", language)
		}
		p[i] = strings.ToLower(p[i])
	}
	return strings.Join(p, "-"), nil
}

// NormalizeLanguage returns the canonical supported locale spelling, including
// Android's historical iw/in/ji aliases.
func NormalizeLanguage(language string) (string, error) { return normalizeLanguage(language) }

func LocaleFolder(language string) (string, error) {
	l, e := normalizeLanguage(language)
	if e != nil {
		return "", e
	}
	p := strings.Split(l, "-")
	if len(p) == 1 && len(p[0]) == 2 {
		return "values-" + l, nil
	}
	if len(p) == 2 && len(p[0]) == 2 && len(p[1]) == 2 && regionRE.MatchString(p[1]) {
		return "values-" + p[0] + "-r" + p[1], nil
	}
	return "values-b+" + strings.Join(p, "+"), nil
}
func LanguageForFolder(folder string) (string, bool) {
	// car is a UI-mode qualifier. The language car requires BCP 47 notation.
	if folder == "values-car" {
		return "", false
	}
	if !strings.HasPrefix(folder, "values-") {
		return "", false
	}
	suffix := strings.TrimPrefix(folder, "values-")
	var l string
	if strings.HasPrefix(suffix, "b+") {
		l = strings.ReplaceAll(strings.TrimPrefix(suffix, "b+"), "+", "-")
	} else {
		p := strings.Split(suffix, "-")
		if len(p) > 2 {
			return "", false
		}
		l = p[0]
		if len(p) == 2 {
			if len(p[1]) != 3 || p[1][0] != 'r' {
				return "", false
			}
			l += "-" + p[1][1:]
		}
	}
	normalized, e := normalizeLanguage(l)
	return normalized, e == nil
}

// safePath rejects symlinks in every existing component, including directories.
func safePath(path string) error {
	abs, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(abs, current), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		s, e := os.Lstat(current)
		if errors.Is(e, os.ErrNotExist) {
			return nil
		}
		if e != nil {
			return e
		}
		if s.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink path %s", current)
		}
	}
	return nil
}
func xmlFiles(dir string) ([]string, error) {
	if e := safePath(dir); e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(dir)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var result []string
	for _, item := range entries {
		if strings.EqualFold(filepath.Ext(item.Name()), ".xml") {
			p := filepath.Join(dir, item.Name())
			if item.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("refusing symlink resource %s", p)
			}
			if !item.IsDir() {
				result = append(result, p)
			}
		}
	}
	return result, nil
}
func FindSources(resourceDir string) ([]string, error) {
	return xmlFiles(filepath.Join(resourceDir, "values"))
}

func entryID(d *document, e Entry) string {
	if e.SkipReason == "array resources are not supported" {
		return "array/" + e.Key
	}
	if e.PluralForms != nil {
		return ResourceID(e.Key, true)
	}
	if _, ok := d.byID[ResourceID(e.Key, false)]; ok {
		return ResourceID(e.Key, false)
	}
	return "array/" + e.Key
}
func mergeDocuments(paths []string) ([]*document, map[string]Entry, []string, error) {
	docs := make([]*document, 0, len(paths))
	entries := map[string]Entry{}
	var warnings []string
	for _, path := range paths {
		d, e := readDocument(path)
		if e != nil {
			return nil, nil, nil, e
		}
		docs = append(docs, d)
		for _, entry := range d.entries {
			id := entryID(d, entry)
			if _, exists := entries[id]; exists {
				return nil, nil, nil, fmt.Errorf("duplicate resource %s in %s", id, path)
			}
			entries[id] = entry
			if entry.SkipReason != "" {
				warnings = append(warnings, fmt.Sprintf("%s: %s: %s", path, id, entry.SkipReason))
			}
		}
	}
	return docs, entries, warnings, nil
}

// Source plurals must supply the mandatory fallback; target plurals may be
// incomplete because filling their missing quantities is this adapter's job.
func validateSourceEntry(entry Entry) Entry {
	if entry.PluralForms != nil && strings.TrimSpace(entry.PluralForms["other"]) == "" && entry.SkipReason == "" {
		entry.SkipReason = "source plural requires a nonempty other item"
	}
	return entry
}
func validateSourceDocuments(docs []*document, entries map[string]Entry) []string {
	var warnings []string
	for _, d := range docs {
		for i, entry := range d.entries {
			updated := validateSourceEntry(entry)
			d.entries[i] = updated
			entries[entryID(d, updated)] = updated
			if updated.SkipReason != entry.SkipReason {
				warnings = append(warnings, fmt.Sprintf("%s: %s: %s", d.path, entryID(d, updated), updated.SkipReason))
			}
		}
	}
	return warnings
}

func resourceRoot(sourcePath string) (string, error) {
	if filepath.Base(filepath.Dir(sourcePath)) != "values" || !strings.EqualFold(filepath.Ext(sourcePath), ".xml") {
		return "", fmt.Errorf("Android source must be res/values/*.xml: %s", sourcePath)
	}
	if e := safePath(sourcePath); e != nil {
		return "", e
	}
	return filepath.Dir(filepath.Dir(sourcePath)), nil
}
func localizedPaths(root, language string) ([]string, error) {
	dirs, e := os.ReadDir(root)
	if e != nil {
		return nil, e
	}
	var paths []string
	for _, dir := range dirs {
		l, ok := LanguageForFolder(dir.Name())
		if !ok || l != language {
			continue
		}
		if dir.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing symlink locale directory %s", dir.Name())
		}
		if !dir.IsDir() {
			continue
		}
		files, e := xmlFiles(filepath.Join(root, dir.Name()))
		if e != nil {
			return nil, e
		}
		paths = append(paths, files...)
	}
	return paths, nil
}
func Load(sourcePath, sourceLanguage string) (*Catalog, error) {
	root, e := resourceRoot(sourcePath)
	if e != nil {
		return nil, e
	}
	sourceLanguage, e = normalizeLanguage(sourceLanguage)
	if e != nil {
		return nil, e
	}
	paths, e := FindSources(root)
	if e != nil {
		return nil, e
	}
	docs, source, warnings, e := mergeDocuments(paths)
	if e != nil {
		return nil, e
	}
	warnings = append(warnings, validateSourceDocuments(docs, source)...)
	c := &Catalog{Localizations: map[string]map[string]Entry{sourceLanguage: source}, Warnings: warnings}
	found := false
	for _, d := range docs {
		if filepath.Clean(d.path) == filepath.Clean(sourcePath) {
			c.Entries = d.entries
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("source XML not found: %s", sourcePath)
	}
	dirs, e := os.ReadDir(root)
	if e != nil {
		return nil, e
	}
	seen := map[string]bool{}
	for _, dir := range dirs {
		language, ok := LanguageForFolder(dir.Name())
		if !ok || seen[language] {
			continue
		}
		seen[language] = true
		files, e := localizedPaths(root, language)
		if e != nil {
			return nil, e
		}
		_, entries, warnings, e := mergeDocuments(files)
		if e != nil {
			return nil, e
		}
		// Default resources define the source language even if an explicit locale exists.
		if language != sourceLanguage {
			c.Localizations[language] = entries
		}
		c.Warnings = append(c.Warnings, warnings...)
	}
	return c, nil
}
