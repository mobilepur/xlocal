package android

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type edit struct {
	start, end  int
	replacement string
}

func replaceContent(d *document, n *node, text string) edit {
	if bytes.HasSuffix(bytes.TrimSpace(d.data[n.start:n.contentStart]), []byte("/>")) {
		opening := string(d.data[n.start:n.contentStart])
		at := strings.LastIndex(opening, "/>")
		return edit{n.start, n.end, opening[:at] + ">" + text + "</" + n.name.Local + ">"}
	}
	return edit{n.contentStart, n.contentEnd, text}
}
func insertChildren(d *document, n *node, text string) edit {
	if bytes.HasSuffix(bytes.TrimSpace(d.data[n.start:n.contentStart]), []byte("/>")) {
		return replaceContent(d, n, text)
	}
	return edit{n.contentEnd, n.contentEnd, text}
}
func renderTranslation(t Translation, source Entry) (string, error) {
	var attrs string
	if !source.Formatted {
		attrs = ` formatted="false"`
	}
	if t.PluralForms == nil {
		text, e := encodeString(t.Text)
		if e != nil {
			return "", e
		}
		return `    <string name="` + t.Key + `"` + attrs + `>` + text + `</string>` + "\n", nil
	}
	var b strings.Builder
	b.WriteString(`    <plurals name="` + t.Key + `"` + attrs + ">\n")
	for _, q := range quantityOrder {
		if text, ok := t.PluralForms[q]; ok {
			encoded, e := encodeString(text)
			if e != nil {
				return "", e
			}
			b.WriteString(`        <item quantity="` + q + `">` + encoded + "</item>\n")
		}
	}
	b.WriteString("    </plurals>\n")
	return b.String(), nil
}
func applyEdits(d *document, edits []edit) ([]byte, error) {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var b bytes.Buffer
	cursor := 0
	for _, e := range edits {
		if e.start < cursor || e.end < e.start || e.end > len(d.data) {
			return nil, errors.New("overlapping Android XML edits")
		}
		b.Write(d.data[cursor:e.start])
		b.WriteString(e.replacement)
		cursor = e.end
	}
	b.Write(d.data[cursor:])
	out := b.Bytes()
	parsed, e := parseDocument(d.path, out)
	if e != nil {
		return nil, e
	}
	for _, entry := range parsed.entries {
		if entry.SkipReason != "" { // Existing unsupported resources remain byte-identical and are allowed.
			originalFound := false
			for _, old := range d.entries {
				if old.Key == entry.Key && old.SkipReason == entry.SkipReason {
					originalFound = true
					break
				}
			}
			if !originalFound {
				return nil, fmt.Errorf("generated unsupported resource %s: %s", entry.Key, entry.SkipReason)
			}
		}
	}
	return out, nil
}

// SaveTranslations preserves existing nonempty translations and patches only
// empty resources or missing plural quantities. All files are validated first.
func SaveTranslations(sourcePath, targetLanguage string, changes []Translation) error {
	root, e := resourceRoot(sourcePath)
	if e != nil {
		return e
	}
	language, e := normalizeLanguage(targetLanguage)
	if e != nil {
		return e
	}
	folder, e := LocaleFolder(language)
	if e != nil {
		return e
	}
	sourcePaths, e := FindSources(root)
	if e != nil {
		return e
	}
	sources, _, _, e := mergeDocuments(sourcePaths)
	if e != nil {
		return e
	}
	sourceEntries := map[string]Entry{}
	for _, d := range sources {
		if filepath.Clean(d.path) == filepath.Clean(sourcePath) {
			for _, entry := range d.entries {
				sourceEntries[entryID(d, entry)] = validateSourceEntry(entry)
			}
		}
	}
	if len(sourceEntries) == 0 && len(changes) > 0 {
		return errors.New("source has no translatable resources")
	}
	paths, e := localizedPaths(root, language)
	if e != nil {
		return e
	}
	docs, existing, _, e := mergeDocuments(paths)
	if e != nil {
		return e
	}
	destinationPath := filepath.Join(root, folder, filepath.Base(sourcePath))
	if e := safePath(destinationPath); e != nil {
		return e
	}
	byPath := map[string]*document{}
	owner := map[string]*document{}
	for _, d := range docs {
		byPath[d.path] = d
		for id := range d.byID {
			owner[id] = d
		}
	}
	patches := map[string][]edit{}
	appendEntries := map[string]string{}
	seen := map[string]bool{}
	for _, change := range changes {
		id := ResourceID(change.Key, change.PluralForms != nil)
		source, ok := sourceEntries[id]
		if !ok {
			return fmt.Errorf("unknown source resource %s", id)
		}
		if seen[id] {
			return fmt.Errorf("duplicate translation %s", id)
		}
		seen[id] = true
		if !source.Translatable || source.SkipReason != "" {
			return fmt.Errorf("resource %s is not eligible for translation", id)
		}
		if change.PluralForms != nil {
			if len(change.PluralForms) == 0 {
				return fmt.Errorf("empty plural translation %s", id)
			}
			for q, text := range change.PluralForms {
				if !quantities[q] || strings.TrimSpace(text) == "" {
					return fmt.Errorf("invalid plural quantity %q for %s", q, id)
				}
				if _, e := encodeString(text); e != nil {
					return e
				}
			}
		} else {
			if strings.TrimSpace(change.Text) == "" {
				return fmt.Errorf("empty translation %s", id)
			}
			if _, e := encodeString(change.Text); e != nil {
				return e
			}
		}
		old, present := existing[id]
		if !present {
			rendered, e := renderTranslation(change, source)
			if e != nil {
				return e
			}
			appendEntries[destinationPath] += rendered
			continue
		}
		if !old.Translatable || old.SkipReason != "" {
			return fmt.Errorf("existing resource %s cannot be safely changed", id)
		}
		d := owner[id]
		n := d.byID[id]
		if change.PluralForms == nil {
			if strings.TrimSpace(old.Text) != "" {
				continue
			}
			if source.Formatted != old.Formatted {
				return fmt.Errorf("formatted attribute mismatch for %s", id)
			}
			encoded, _ := encodeString(change.Text)
			patches[d.path] = append(patches[d.path], replaceContent(d, n, encoded))
			continue
		}
		var additional strings.Builder
		for _, q := range quantityOrder {
			text, requested := change.PluralForms[q]
			if !requested || strings.TrimSpace(old.PluralForms[q]) != "" {
				continue
			}
			if source.Formatted != old.Formatted {
				return fmt.Errorf("formatted attribute mismatch for %s", id)
			}
			encoded, _ := encodeString(text)
			var found *node
			for _, child := range n.children {
				cq, _ := child.attr("quantity")
				if cq == q {
					found = child
					break
				}
			}
			if found != nil {
				patches[d.path] = append(patches[d.path], replaceContent(d, found, encoded))
			} else {
				additional.WriteString(`        <item quantity="` + q + `">` + encoded + "</item>\n")
			}
		}
		if additional.Len() > 0 {
			patches[d.path] = append(patches[d.path], insertChildren(d, n, "\n"+additional.String()+"    "))
		}
	}
	for path, text := range appendEntries {
		d := byPath[path]
		if d == nil {
			d, e = parseDocument(path, []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n</resources>\n"))
			if e != nil {
				return e
			}
			byPath[path] = d
		}
		patches[path] = append(patches[path], insertChildren(d, d.root, "\n"+text))
	}
	// Build and parse every proposed file before creating any directories or files.
	outputs := map[string][]byte{}
	for path, edits := range patches {
		out, e := applyEdits(byPath[path], edits)
		if e != nil {
			return e
		}
		outputs[path] = out
	}
	paths = paths[:0]
	for path := range outputs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if e := safePath(path); e != nil {
			return e
		}
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			return e
		}
		if e := safePath(path); e != nil {
			return e
		}
		mode := os.FileMode(0644)
		if stat, e := os.Stat(path); e == nil {
			mode = stat.Mode().Perm()
		}
		tmp, e := os.CreateTemp(filepath.Dir(path), ".xlocal-*.xml")
		if e != nil {
			return e
		}
		tempName := tmp.Name()
		e = tmp.Chmod(mode)
		if e == nil {
			_, e = tmp.Write(outputs[path])
		}
		if e == nil {
			e = tmp.Sync()
		}
		closeErr := tmp.Close()
		if e == nil {
			e = closeErr
		}
		if e == nil {
			e = safePath(path)
		}
		if e == nil {
			e = os.Rename(tempName, path)
		}
		if e != nil {
			os.Remove(tempName)
			return e
		}
	}
	return nil
}
