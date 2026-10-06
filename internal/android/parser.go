package android

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Recognize actual placeholder shapes without treating arbitrary braces as
// templates. These require a MessageFormat-specific validator before translation.
var messageFormatRE = regexp.MustCompile(`\{\s*(?:[\pL_][\pL\pN_.-]*|[0-9]+)\s*(?:\}|,\s*(?:plural|selectordinal|select|number|date|time|choice|ordinal|spellout|duration)\s*[,}])`)

// node stores byte spans so edits preserve comments, attributes and formatting.
type node struct {
	name                                 xml.Name
	attrs                                []xml.Attr
	start, contentStart, contentEnd, end int
	text                                 string
	children                             []*node
	comment                              string
}
type document struct {
	path    string
	data    []byte
	root    *node
	entries []Entry
	byID    map[string]*node
}

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.attrs {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}
func parseDocument(path string, data []byte) (*document, error) {
	d := &document{path: path, data: data, byID: map[string]*node{}}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack []*node
	pendingComment := ""
	for {
		before := int(decoder.InputOffset())
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, fmt.Errorf("%s: %w", path, e)
		}
		after := int(decoder.InputOffset())
		switch t := token.(type) {
		case xml.StartElement:
			seenAttrs := map[xml.Name]bool{}
			for _, attr := range t.Attr {
				if seenAttrs[attr.Name] {
					return nil, fmt.Errorf("%s: duplicate XML attribute %s", path, attr.Name.Local)
				}
				seenAttrs[attr.Name] = true
			}
			n := &node{name: t.Name, attrs: t.Attr, start: before, contentStart: after}
			if len(stack) == 0 {
				if d.root != nil || t.Name.Local != "resources" || t.Name.Space != "" {
					return nil, fmt.Errorf("%s: expected a single resources root", path)
				}
				d.root = n
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
				if len(stack) == 1 {
					n.comment = strings.TrimSpace(pendingComment)
					pendingComment = ""
				}
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("%s: unexpected closing element", path)
			}
			n := stack[len(stack)-1]
			n.contentEnd = before
			n.end = after
			if n.contentEnd < n.contentStart {
				n.contentEnd = n.contentStart
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(t)) != "" {
					return nil, fmt.Errorf("%s: text outside resources", path)
				}
			} else {
				stack[len(stack)-1].text += string(t)
				if len(stack) == 1 && strings.TrimSpace(string(t)) != "" {
					pendingComment = ""
				}
			}
		case xml.Comment:
			if len(stack) == 1 {
				if pendingComment != "" {
					pendingComment += "\n"
				}
				pendingComment += string(t)
			}
		case xml.Directive:
			return nil, fmt.Errorf("%s: XML directives are unsupported", path)
		}
	}
	if d.root == nil || len(stack) != 0 {
		return nil, fmt.Errorf("%s: incomplete resources XML", path)
	}
	if strings.TrimSpace(d.root.text) != "" {
		return nil, fmt.Errorf("%s: text outside resource elements", path)
	}
	for _, n := range d.root.children {
		if n.name.Local != "string" && n.name.Local != "plurals" && n.name.Local != "string-array" && n.name.Local != "array" {
			continue
		}
		key, ok := n.attr("name")
		if !ok || !keyRE.MatchString(key) {
			return nil, fmt.Errorf("%s: invalid resource name %q", path, key)
		}
		plural := n.name.Local == "plurals"
		id := ResourceID(key, plural)
		if n.name.Local == "array" || n.name.Local == "string-array" {
			id = "array/" + key
		}
		if _, exists := d.byID[id]; exists {
			return nil, fmt.Errorf("%s: duplicate resource %s", path, id)
		}
		d.byID[id] = n
		entry := Entry{Key: key, Comment: n.comment, Formatted: true, Translatable: true}
		for _, a := range n.attrs {
			if a.Name.Space != "" {
				if a.Name.Space != "xmlns" {
					entry.SkipReason = "namespaced resource attributes are unsupported"
				}
				continue
			}
			switch a.Name.Local {
			case "name":
			case "translatable":
				if a.Value == "false" {
					entry.Translatable = false
				} else if a.Value != "true" {
					return nil, fmt.Errorf("%s: invalid translatable value", path)
				}
			case "formatted":
				if a.Value == "false" {
					entry.Formatted = false
				} else if a.Value != "true" {
					return nil, fmt.Errorf("%s: invalid formatted value", path)
				}
			default:
				entry.SkipReason = "resource attributes other than name, formatted and translatable are unsupported"
			}
		}
		if n.name.Space != "" {
			entry.SkipReason = "namespaced resources are unsupported"
		}
		if n.name.Local == "array" || n.name.Local == "string-array" {
			entry.SkipReason = "array resources are not supported"
			d.entries = append(d.entries, entry)
			continue
		}
		if !plural {
			if len(n.children) > 0 {
				entry.SkipReason = "styled or nested string content is unsupported"
			} else {
				text, e := decodeString(n.text)
				if e != nil {
					entry.SkipReason = e.Error()
				} else {
					entry.Text = text
				}
			}
		} else {
			entry.PluralForms = map[string]string{}
			if strings.TrimSpace(n.text) != "" {
				entry.SkipReason = "text outside plural items is unsupported"
			}
			for _, item := range n.children {
				q, ok := item.attr("quantity")
				if item.name.Local != "item" || item.name.Space != "" || !ok || !quantities[q] {
					entry.SkipReason = "unsupported plural item"
					continue
				}
				if _, exists := entry.PluralForms[q]; exists {
					return nil, fmt.Errorf("%s: duplicate quantity %s in %s", path, q, key)
				}
				if len(item.attrs) != 1 || len(item.children) > 0 {
					entry.SkipReason = "styled or attributed plural items are unsupported"
				}
				text, e := decodeString(item.text)
				if e != nil {
					entry.SkipReason = e.Error()
				}
				entry.PluralForms[q] = text
			}
		}
		if !plural && isAlias(n.text) {
			entry.SkipReason = "resource aliases are unsupported"
		}
		for _, child := range n.children {
			if isAlias(child.text) {
				entry.SkipReason = "resource aliases are unsupported"
			}
		}
		if entry.SkipReason == "" {
			if messageFormatRE.MatchString(entry.Text) {
				entry.SkipReason = "ICU/MessageFormat or named placeholders are unsupported"
			}
			for _, text := range entry.PluralForms {
				if messageFormatRE.MatchString(text) {
					entry.SkipReason = "ICU/MessageFormat or named placeholders are unsupported"
					break
				}
			}
		}
		d.entries = append(d.entries, entry)
	}
	return d, nil
}
func readDocument(path string) (*document, error) {
	if e := safePath(path); e != nil {
		return nil, e
	}
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	return parseDocument(path, data)
}
func isAlias(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "@") || strings.HasPrefix(text, "?")
}
