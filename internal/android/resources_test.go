package android

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, rel, body string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "res", rel)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}
func put(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, p string) string {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestLocaleFolders(t *testing.T) {
	for _, tt := range []struct{ lang, folder string }{{"de", "values-de"}, {"pt-BR", "values-pt-rBR"}, {"zh-Hant", "values-b+zh+Hant"}, {"es-419", "values-b+es+419"}} {
		f, e := LocaleFolder(tt.lang)
		if e != nil || f != tt.folder {
			t.Errorf("%s: %q %v", tt.lang, f, e)
		}
		l, ok := LanguageForFolder(tt.folder)
		if !ok || l != tt.lang {
			t.Errorf("reverse %s: %q %v", tt.folder, l, ok)
		}
	}
	for _, bad := range []string{"../de", "de/../../x", "", "values-de", "de--DE"} {
		if _, e := LocaleFolder(bad); e == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
func TestLoadEscapesAndUnsupported(t *testing.T) {
	p := fixture(t, "values/messages.xml", `<resources><!-- Greeting context --><string name="hello">"  A &amp; B\nIt\'s \\fine  "</string><string name="styled">A <b>bold</b></string><string name="alias">@string/hello</string><string name="brand" translatable="false">Brand</string><plurals name="songs"><item quantity="one">%d song</item><item quantity="other">%d songs</item></plurals><string-array name="array"><item>A</item></string-array></resources>`)
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if c == nil || len(c.Entries) != 6 {
		t.Fatalf("entries: %#v", c)
	}
	if c.Entries[0].Text != "  A & B\nIt's \\fine  " || c.Entries[0].Comment != "Greeting context" {
		t.Errorf("decoded %#v", c.Entries[0])
	}
	if c.Entries[1].SkipReason == "" || c.Entries[2].SkipReason == "" || c.Entries[3].Translatable || len(c.Warnings) == 0 {
		t.Errorf("unsupported flags %#v", c)
	}
}
func TestSavePreservesBytesAndExistingOtherFile(t *testing.T) {
	p := fixture(t, "values/messages.xml", `<resources><string name="hello">Hello</string><string name="second">Second</string></resources>`)
	root := filepath.Dir(filepath.Dir(p))
	other := filepath.Join(root, "values-de", "other.xml")
	original := "<?xml version=\"1.0\"?>\n<resources>\n <!-- Keep this exactly -->\n <color name=\"accent\">#fff</color>\n <string name=\"hello\">Hallo</string>\n</resources>\n"
	put(t, other, original)
	if e := SaveTranslations(p, "de", []Translation{{Key: "hello", Text: "Overwrite"}, {Key: "second", Text: "  Zwei & drei\n\"O'Neil\" \\ "}}); e != nil {
		t.Fatal(e)
	}
	if got := read(t, other); got != original {
		t.Fatalf("existing changed: %q", got)
	}
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if c.Localizations["de"]["string/hello"].Text != "Hallo" || c.Localizations["de"]["string/second"].Text != "  Zwei & drei\n\"O'Neil\" \\ " {
		t.Fatalf("roundtrip %#v", c.Localizations)
	}
}
func TestFillPluralAndEmptyString(t *testing.T) {
	p := fixture(t, "values/strings.xml", `<resources><plurals name="count"><item quantity="one">One</item><item quantity="other">Many</item></plurals><string name="empty">Empty</string></resources>`)
	dest := filepath.Join(filepath.Dir(filepath.Dir(p)), "values-de", "strings.xml")
	before := `<resources><!-- prefix --><plurals name="count"><item quantity="one">Eins</item><!-- middle --><item quantity="other"/></plurals><string name="empty"/><!-- suffix --></resources>`
	put(t, dest, before)
	if e := SaveTranslations(p, "de", []Translation{{Key: "count", PluralForms: map[string]string{"one": "Changed", "other": "Viele"}}, {Key: "empty", Text: "Leer"}}); e != nil {
		t.Fatal(e)
	}
	got := read(t, dest)
	for _, keep := range []string{`<item quantity="one">Eins</item>`, `<!-- prefix -->`, `<!-- middle -->`, `<!-- suffix -->`} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %s: %s", keep, got)
		}
	}
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if c.Localizations["de"]["plurals/count"].PluralForms["other"] != "Viele" || c.Localizations["de"]["string/empty"].Text != "Leer" {
		t.Fatalf("fill %#v", c)
	}
}
func TestRejectBeforeWrite(t *testing.T) {
	p := fixture(t, "values/strings.xml", `<resources><string name="valid">Valid</string></resources>`)
	for _, changes := range [][]Translation{{{Key: "valid", Text: "Good"}, {Key: "unknown", Text: "Bad"}}, {{Key: "valid", Text: "NUL\x00"}}} {
		if e := SaveTranslations(p, "de", changes); e == nil {
			t.Errorf("accepted invalid %#v", changes)
		}
		if _, e := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(p)), "values-de")); !os.IsNotExist(e) {
			t.Errorf("wrote invalid response: %v", e)
		}
	}
}
func TestDuplicateAcrossFilesAndSymlink(t *testing.T) {
	p := fixture(t, "values/strings.xml", `<resources><string name="valid">Valid</string></resources>`)
	root := filepath.Dir(filepath.Dir(p))
	put(t, filepath.Join(root, "values", "other.xml"), `<resources><string name="valid">Duplicate</string></resources>`)
	if _, e := Load(p, "en"); e == nil {
		t.Error("accepted duplicate sources")
	}
	if e := os.Remove(filepath.Join(root, "values", "other.xml")); e != nil {
		t.Fatal(e)
	}
	external := t.TempDir()
	if e := os.Symlink(external, filepath.Join(root, "values-de")); e != nil {
		t.Fatal(e)
	}
	if e := SaveTranslations(p, "de", []Translation{{Key: "valid", Text: "Hallo"}}); e == nil {
		t.Error("followed symlink destination")
	}
}

func TestSelfClosingRootAndPluralAppend(t *testing.T) {
	p := fixture(t, "values/misc.xml", `<resources><plurals name="people"><item quantity="other">People</item></plurals><string name="literal" formatted="false">100% ready</string></resources>`)
	root := filepath.Dir(filepath.Dir(p))
	dest := filepath.Join(root, "values-de", "misc.xml")
	put(t, dest, `<?xml version="1.0"?><resources />`)
	changes := []Translation{{Key: "people", PluralForms: map[string]string{"one": "Person", "other": "People"}}, {Key: "literal", Text: "100% bereit"}}
	if e := SaveTranslations(p, "de", changes); e != nil {
		t.Fatal(e)
	}
	got := read(t, dest)
	if !strings.Contains(got, `formatted="false"`) {
		t.Fatal(got)
	}
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if c.Localizations["de"]["string/literal"].Formatted || c.Localizations["de"]["plurals/people"].PluralForms["one"] != "Person" {
		t.Fatalf("roundtrip %#v", c)
	}
	before := got
	if e := SaveTranslations(p, "de", changes); e != nil {
		t.Fatal(e)
	}
	if read(t, dest) != before {
		t.Error("second save changed existing bytes")
	}
}
func TestPluralAppendPreservesOriginal(t *testing.T) {
	p := fixture(t, "values/misc.xml", `<resources><plurals name="people"><item quantity="other">People</item></plurals></resources>`)
	dest := filepath.Join(filepath.Dir(filepath.Dir(p)), "values-ru", "misc.xml")
	before := `<resources><plurals name="people"><!-- keep --><item quantity="one">Человек</item></plurals></resources>`
	put(t, dest, before)
	if e := SaveTranslations(p, "ru", []Translation{{Key: "people", PluralForms: map[string]string{"one": "Overwrite", "few": "Люди", "many": "Люди", "other": "Люди"}}}); e != nil {
		t.Fatal(e)
	}
	got := read(t, dest)
	if !strings.Contains(got, `<plurals name="people"><!-- keep --><item quantity="one">Человек</item>`) {
		t.Fatal(got)
	}
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Localizations["ru"]["plurals/people"].PluralForms) != 4 {
		t.Fatalf("forms %#v", c.Localizations["ru"])
	}
}
func TestDuplicateDestinationAndMalformedXMLNeverWrite(t *testing.T) {
	for _, bad := range []string{`<resources><string name="hello">Hallo</string><string name="hello">Again</string></resources>`, `<resources><string name="hello">Unclosed</resources>`, `<resources><string name="hello" name="other">Duplicate attribute</string></resources>`} {
		p := fixture(t, "values/a.xml", `<resources><string name="hello">Hello</string></resources>`)
		dest := filepath.Join(filepath.Dir(filepath.Dir(p)), "values-de", "a.xml")
		put(t, dest, bad)
		if e := SaveTranslations(p, "de", []Translation{{Key: "hello", Text: "Hallo"}}); e == nil {
			t.Errorf("accepted %s", bad)
		}
		if read(t, dest) != bad {
			t.Error("invalid destination changed")
		}
	}
	p := fixture(t, "values/a.xml", `<resources><string name="hello">Hello</string></resources>`)
	root := filepath.Dir(filepath.Dir(p))
	put(t, filepath.Join(root, "values-de", "a.xml"), `<resources><string name="hello">Hallo</string></resources>`)
	put(t, filepath.Join(root, "values-de", "b.xml"), `<resources><string name="hello">Again</string></resources>`)
	if e := SaveTranslations(p, "de", []Translation{{Key: "hello", Text: "Hallo"}}); e == nil {
		t.Error("accepted cross-file target duplicate")
	}
}
func TestEscapingRoundTripAndLiteralReferences(t *testing.T) {
	for _, text := range []string{"  leading and trailing  ", "O'Neil says \"Hi\"", "a\\b\n\t\r<> & 😀", "@literal", "?literal", "%1$s", "\u00a0two\u00a0"} {
		encoded, e := encodeString(text)
		if e != nil {
			t.Fatal(e)
		}
		d, e := parseDocument("test.xml", []byte(`<resources><string name="test">`+encoded+`</string></resources>`))
		if e != nil {
			t.Fatal(e)
		}
		entry := d.entries[0]
		if entry.Text != text || entry.SkipReason != "" {
			t.Errorf("%q -> %q -> %#v", text, encoded, entry)
		}
	}
	for _, tt := range []struct{ source, want string }{{"  A\n B\tC  ", "A B C"}, {`\u0041`, "A"}, {`\@string/foo`, "@string/foo"}, {`"  A " B`, "  A  B"}} {
		got, e := decodeString(tt.source)
		if e != nil || got != tt.want {
			t.Errorf("%q: %q %v want %q", tt.source, got, e, tt.want)
		}
	}
}
func TestNamespaceKindsAndQualifiedDirectories(t *testing.T) {
	p := fixture(t, "values/a.xml", `<resources><string name="same">Text</string><plurals name="same"><item quantity="other">Texts</item></plurals><string-array name="same"><item>Array</item></string-array></resources>`)
	root := filepath.Dir(filepath.Dir(p))
	put(t, filepath.Join(root, "values-night", "a.xml"), `<resources><string name="same">Night</string></resources>`)
	put(t, filepath.Join(root, "values-de-rDE-night", "a.xml"), `<resources><string name="same">Night DE</string></resources>`)
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Localizations["en"]) != 3 || len(c.Localizations) != 1 {
		t.Fatalf("namespace %#v", c.Localizations)
	}
	sources, e := FindSources(root)
	if e != nil || len(sources) != 1 {
		t.Fatalf("sources %v %v", sources, e)
	}
}
func TestUnsupportedResponseNeverWrites(t *testing.T) {
	p := fixture(t, "values/a.xml", `<resources><string name="styled"><b>Hello</b></string><string name="locked" translatable="false">Brand</string><string name="ok">OK</string><plurals name="count"><item quantity="other">Things</item></plurals></resources>`)
	for _, changes := range [][]Translation{{{Key: "styled", Text: "Hallo"}}, {{Key: "locked", Text: "Marke"}}, {{Key: "count", PluralForms: map[string]string{"invalid": "Bad"}}}, {{Key: "ok", Text: "Gut"}, {Key: "ok", Text: "Again"}}} {
		if e := SaveTranslations(p, "de", changes); e == nil {
			t.Errorf("accepted %#v", changes)
		}
		if _, e := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(p)), "values-de")); !os.IsNotExist(e) {
			t.Errorf("created destination: %v", e)
		}
	}
}

// Set XLOCAL_AAPT2 to a local SDK executable to verify generated resource syntax
// with the Android compiler without installing tools or accessing the network.
func TestGeneratedXMLCompilesWithAAPT2(t *testing.T) {
	aapt := os.Getenv("XLOCAL_AAPT2")
	if aapt == "" {
		var e error
		aapt, e = exec.LookPath("aapt2")
		if e != nil {
			t.Skip("AAPT2 unavailable; set XLOCAL_AAPT2 for Android compiler verification")
		}
	}
	p := fixture(t, "values/messages.xml", `<resources><string name="message">Message</string><plurals name="songs"><item quantity="one">%1$d song</item><item quantity="other">%1$d songs</item></plurals><string name="discount" formatted="false">100% ready</string></resources>`)
	changes := []Translation{{Key: "message", Text: "  O'Neil &amp; \"Hallo\" \\ 😀\n  "}, {Key: "songs", PluralForms: map[string]string{"one": "%1$d Lied", "other": "%1$d Lieder"}}, {Key: "discount", Text: "100% bereit"}}
	if e := SaveTranslations(p, "de", changes); e != nil {
		t.Fatal(e)
	}
	root := filepath.Dir(filepath.Dir(p))
	cmd := exec.Command(aapt, "compile", "--dir", root, "-o", filepath.Join(t.TempDir(), "resources.zip"))
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("AAPT2 compile: %v\n%s", e, out)
	}
}

func TestSourcePluralWithoutOtherIsUnsupported(t *testing.T) {
	p := fixture(t, "values/a.xml", `<resources><plurals name="invalid"><item quantity="one">One</item></plurals><plurals name="empty"><item quantity="other"> </item></plurals></resources>`)
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Warnings) != 2 || c.Entries[0].SkipReason == "" || c.Entries[1].SkipReason == "" {
		t.Fatalf("missing unsupported diagnostics: %#v", c)
	}
	if e := SaveTranslations(p, "de", []Translation{{Key: "invalid", PluralForms: map[string]string{"other": "Dinge"}}}); e == nil {
		t.Error("wrote invalid source plural")
	}
}
func TestFormattedMismatchBeforeEmptyFill(t *testing.T) {
	p := fixture(t, "values/a.xml", `<resources><string name="literal" formatted="false">100% ready</string><plurals name="count"><item quantity="other">%d items</item></plurals></resources>`)
	dest := filepath.Join(filepath.Dir(filepath.Dir(p)), "values-de", "a.xml")
	before := `<resources><string name="literal"/><plurals name="count" formatted="false"><item quantity="one">Ein Ding</item></plurals></resources>`
	put(t, dest, before)
	for _, changes := range [][]Translation{{{Key: "literal", Text: "100% bereit"}}, {{Key: "count", PluralForms: map[string]string{"other": "%d Dinge"}}}} {
		if e := SaveTranslations(p, "de", changes); e == nil {
			t.Errorf("accepted formatting mismatch %#v", changes)
		}
		if read(t, dest) != before {
			t.Error("changed mismatch destination")
		}
	}
	put(t, dest, `<resources><string name="literal">Keep existing</string></resources>`)
	if e := SaveTranslations(p, "de", []Translation{{Key: "literal", Text: "100% bereit"}}); e != nil {
		t.Errorf("nonempty mismatch should preserve existing: %v", e)
	}
}
func TestWhitespaceOnlyTranslationIsFilled(t *testing.T) {
	p := fixture(t, "values/a.xml", `<resources><string name="text">Text</string><plurals name="count"><item quantity="other">Items</item></plurals></resources>`)
	dest := filepath.Join(filepath.Dir(filepath.Dir(p)), "values-de", "a.xml")
	put(t, dest, `<resources><string name="text">"  "</string><plurals name="count"><item quantity="other">"  "</item></plurals></resources>`)
	if e := SaveTranslations(p, "de", []Translation{{Key: "text", Text: "Text DE"}, {Key: "count", PluralForms: map[string]string{"other": "Dinge"}}}); e != nil {
		t.Fatal(e)
	}
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if c.Localizations["de"]["string/text"].Text != "Text DE" || c.Localizations["de"]["plurals/count"].PluralForms["other"] != "Dinge" {
		t.Fatalf("not filled: %#v", c.Localizations)
	}
}

func TestLegacyLocaleAliases(t *testing.T) {
	for _, tt := range []struct{ legacy, modern string }{{"iw", "he"}, {"in", "id"}, {"ji", "yi"}, {"DE", "de"}} {
		l, e := NormalizeLanguage(tt.legacy)
		if e != nil || l != tt.modern {
			t.Errorf("normalize %s: %s %v", tt.legacy, l, e)
		}
		if l, ok := LanguageForFolder("values-" + tt.legacy); !ok || l != tt.modern {
			t.Errorf("folder %s: %s %v", tt.legacy, l, ok)
		}
	}
	p := fixture(t, "values/a.xml", `<resources><string name="hello">Hello</string></resources>`)
	root := filepath.Dir(filepath.Dir(p))
	dest := filepath.Join(root, "values-iw", "legacy.xml")
	original := `<resources><string name="hello">שלום</string></resources>`
	put(t, dest, original)
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	if c.Localizations["he"]["string/hello"].Text != "שלום" {
		t.Fatal(c.Localizations)
	}
	if e := SaveTranslations(p, "he", []Translation{{Key: "hello", Text: "Overwrite"}}); e != nil {
		t.Fatal(e)
	}
	if read(t, dest) != original {
		t.Error("changed legacy resource")
	}
	if _, e := os.Stat(filepath.Join(root, "values-he")); !os.IsNotExist(e) {
		t.Errorf("created duplicate canonical folder: %v", e)
	}
}

func TestMessageFormatResourcesAreSkipped(t *testing.T) {
	for _, text := range []string{`Hello {name}`, `Item {0}`, `{count, plural, one {One} other {Many}}`, `{gender, select, male {He} other {They}}`, `{position, selectordinal, one {#st} other {#th}}`, `{0,number,#.##}`, `{0,date,short}`, `{user.name}`} {
		p := fixture(t, "values/a.xml", `<resources><string name="template">`+text+`</string><plurals name="count"><item quantity="other">`+text+`</item></plurals></resources>`)
		c, e := Load(p, "en")
		if e != nil {
			t.Fatal(e)
		}
		if len(c.Warnings) != 2 || !strings.Contains(c.Entries[0].SkipReason, "MessageFormat") || !strings.Contains(c.Entries[1].SkipReason, "MessageFormat") {
			t.Errorf("unprotected template %q: %#v", text, c)
		}
		if e := SaveTranslations(p, "de", []Translation{{Key: "template", Text: "Translated"}}); e == nil {
			t.Errorf("allowed template %q", text)
		}
	}
	for _, text := range []string{`Use { braces with spaces }`, `Empty {}`, `{a + b}`, `%1$s has braces { }`} {
		p := fixture(t, "values/a.xml", `<resources><string name="literal">`+text+`</string></resources>`)
		c, e := Load(p, "en")
		if e != nil {
			t.Fatal(e)
		}
		if c.Entries[0].SkipReason != "" {
			t.Errorf("skipped literal braces %q: %s", text, c.Entries[0].SkipReason)
		}
	}
}

func TestGeneratedRuntimeStringsWithAAPT2(t *testing.T) {
	aapt := os.Getenv("XLOCAL_AAPT2")
	jar := os.Getenv("XLOCAL_ANDROID_JAR")
	if aapt == "" || jar == "" {
		t.Skip("set XLOCAL_AAPT2 and XLOCAL_ANDROID_JAR for compiled runtime-string verification")
	}
	p := fixture(t, "values/a.xml", `<resources><string name="quoted">Quoted</string><string name="at">At</string><string name="question">Question</string><string name="slash">Slash</string><string name="whitespace">Whitespace</string><string name="unicode">Unicode</string><string name="carriage">Carriage</string></resources>`)
	changes := []Translation{{Key: "quoted", Text: `O'Neil says "Hi"`}, {Key: "at", Text: "@string/literal"}, {Key: "question", Text: "?attr/literal"}, {Key: "slash", Text: `C:\new\path`}, {Key: "whitespace", Text: "  spaces\n\t end  "}, {Key: "unicode", Text: "😀 & < >"}, {Key: "carriage", Text: "A\rB"}}
	if e := SaveTranslations(p, "de", changes); e != nil {
		t.Fatal(e)
	}
	root := filepath.Dir(filepath.Dir(p))
	workspace := filepath.Dir(root)
	compiled := filepath.Join(workspace, "resources.zip")
	manifest := filepath.Join(workspace, "AndroidManifest.xml")
	apk := filepath.Join(workspace, "test.apk")
	put(t, manifest, `<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.example.xlocal"><application/></manifest>`)
	for _, args := range [][]string{{"compile", "--dir", root, "-o", compiled}, {"link", "-I", jar, "--manifest", manifest, "-o", apk, compiled}} {
		cmd := exec.Command(aapt, args...)
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("AAPT2 %v: %v\n%s", args, e, out)
		}
	}
	out, e := exec.Command(aapt, "dump", "resources", apk).CombinedOutput()
	if e != nil {
		t.Fatalf("dump: %v\n%s", e, out)
	}
	dump := string(out)
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	for _, change := range changes {
		// AAPT2's printer prefixes continuation lines with the resource indentation.
		expectedDump := `(de) "` + strings.ReplaceAll(change.Text, "\n", "\n      ") + `"`
		resourceAt := strings.Index(dump, "string/"+change.Key+"\n")
		if resourceAt < 0 {
			t.Errorf("missing compiled resource %s", change.Key)
			continue
		}
		block := dump[resourceAt:]
		if next := strings.Index(block, "\n    resource "); next >= 0 {
			block = block[:next]
		}
		if !strings.Contains(block, expectedDump) {
			t.Errorf("compiled %s differs: expected %q in %q", change.Key, expectedDump, block)
		}
		if decoded := c.Localizations["de"][ResourceID(change.Key, false)].Text; decoded != change.Text {
			t.Errorf("adapter %s differs: %q != %q", change.Key, decoded, change.Text)
		}
	}
}

func TestSourceWhitespaceMatchesAAPT2(t *testing.T) {
	aapt := os.Getenv("XLOCAL_AAPT2")
	jar := os.Getenv("XLOCAL_ANDROID_JAR")
	if aapt == "" || jar == "" {
		t.Skip("set XLOCAL_AAPT2 and XLOCAL_ANDROID_JAR for compiled source-string verification")
	}
	tests := []struct {
		name      string
		character rune
		collapse  bool
	}{{"space", ' ', true}, {"tab", '\t', true}, {"lf", '\n', true}, {"cr", '\r', true}, {"nbsp", '\u00a0', false}, {"thin", '\u2009', false}, {"em", '\u2003', false}, {"nextline", '\u0085', false}, {"ogham", '\u1680', false}, {"mongolian", '\u180e', false}, {"linesep", '\u2028', false}, {"paragraphsep", '\u2029', false}, {"narrownbsp", '\u202f', false}, {"mediumspace", '\u205f', false}, {"ideographic", '\u3000', false}}
	var body strings.Builder
	body.WriteString("<resources>")
	for _, tt := range tests {
		body.WriteString(`<string name="` + tt.name + `">` + "A" + strings.Repeat(string(tt.character), 2) + "B" + `</string>`)
	}
	body.WriteString("</resources>")
	p := fixture(t, "values/a.xml", body.String())
	root := filepath.Dir(filepath.Dir(p))
	workspace := filepath.Dir(root)
	compiled := filepath.Join(workspace, "resources.zip")
	manifest := filepath.Join(workspace, "AndroidManifest.xml")
	apk := filepath.Join(workspace, "test.apk")
	put(t, manifest, `<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.example.xlocal"><application/></manifest>`)
	for _, args := range [][]string{{"compile", "--dir", root, "-o", compiled}, {"link", "-I", jar, "--manifest", manifest, "-o", apk, compiled}} {
		if out, e := exec.Command(aapt, args...).CombinedOutput(); e != nil {
			t.Fatalf("AAPT2 %v: %v\n%s", args, e, out)
		}
	}
	out, e := exec.Command(aapt, "dump", "resources", apk).CombinedOutput()
	if e != nil {
		t.Fatalf("dump: %v\n%s", e, out)
	}
	dump := string(out)
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	for _, tt := range tests {
		expected := "A" + strings.Repeat(string(tt.character), 2) + "B"
		if tt.collapse {
			expected = "A B"
		}
		resourceAt := strings.Index(dump, "string/"+tt.name+"\n")
		if resourceAt < 0 {
			t.Errorf("missing compiled resource %s", tt.name)
			continue
		}
		block := dump[resourceAt:]
		if next := strings.Index(block, "\n    resource "); next >= 0 {
			block = block[:next]
		}
		if !strings.Contains(block, `() "`+expected+`"`) {
			t.Errorf("AAPT2 U+%04X expected %q in %q", tt.character, expected, block)
		}
		if got := c.Localizations["en"][ResourceID(tt.name, false)].Text; got != expected {
			t.Errorf("adapter U+%04X: %q; AAPT2 preserves %q", tt.character, got, expected)
		}
	}
}

func TestSourceEscapesAndQuoteBoundariesMatchAAPT2(t *testing.T) {
	aapt := os.Getenv("XLOCAL_AAPT2")
	jar := os.Getenv("XLOCAL_ANDROID_JAR")
	if aapt == "" || jar == "" {
		t.Skip("set XLOCAL_AAPT2 and XLOCAL_ANDROID_JAR for compiled source-string verification")
	}
	tests := []struct {
		name, source, expected string
		skip                   bool
	}{{"slash_r", `"A\rB"`, "ArB", false}, {"unicode_cr", `"A\u000dB"`, "A\rB", false}, {"surrogate", `\uD83D\uDE00`, "", true}, {"native_emoji", "😀", "😀", false}, {"leading", " \t A", "A", false}, {"trailing", "A \t ", "A", false}, {"quote_transition", `A " B " C`, "A  B  C", false}, {"quote_end", `" A "  `, " A ", false}, {"quote_begin", `"  " A`, "   A", false}, {"quote_middle", `A  "  B"`, "A   B", false}, {"lead_surround", ` A "B" C `, "A B C", false}}
	var body strings.Builder
	body.WriteString("<resources>")
	for _, tt := range tests {
		body.WriteString(`<string name="` + tt.name + `">` + tt.source + `</string>`)
	}
	body.WriteString("</resources>")
	p := fixture(t, "values/a.xml", body.String())
	root := filepath.Dir(filepath.Dir(p))
	workspace := filepath.Dir(root)
	compiled := filepath.Join(workspace, "resources.zip")
	manifest := filepath.Join(workspace, "AndroidManifest.xml")
	apk := filepath.Join(workspace, "test.apk")
	put(t, manifest, `<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.example.xlocal"><application/></manifest>`)
	for _, args := range [][]string{{"compile", "--dir", root, "-o", compiled}, {"link", "-I", jar, "--manifest", manifest, "-o", apk, compiled}} {
		if out, e := exec.Command(aapt, args...).CombinedOutput(); e != nil {
			t.Fatalf("AAPT2 %v: %v\n%s", args, e, out)
		}
	}
	out, e := exec.Command(aapt, "dump", "resources", apk).CombinedOutput()
	if e != nil {
		t.Fatalf("dump: %v\n%s", e, out)
	}
	dump := string(out)
	c, e := Load(p, "en")
	if e != nil {
		t.Fatal(e)
	}
	for _, tt := range tests {
		resourceAt := strings.Index(dump, "string/"+tt.name+"\n")
		if resourceAt < 0 {
			t.Errorf("missing compiled resource %s", tt.name)
			continue
		}
		block := dump[resourceAt:]
		if next := strings.Index(block, "\n    resource "); next >= 0 {
			block = block[:next]
		}
		if !strings.Contains(block, `() "`+tt.expected+`"`) {
			t.Errorf("AAPT2 %s expected %q in %q", tt.name, tt.expected, block)
		}
		entry := c.Localizations["en"][ResourceID(tt.name, false)]
		if tt.skip {
			if entry.SkipReason == "" {
				t.Errorf("%s must be reported unsupported: %#v", tt.name, entry)
			}
		} else if entry.Text != tt.expected {
			t.Errorf("adapter %s: %q != compiled %q", tt.name, entry.Text, tt.expected)
		}
	}
}

func TestSurrogateEscapesAreReportedUnsupported(t *testing.T) {
	for _, source := range []string{`\uD83D\uDE00`, `\uD83D`, `\uDE00`} {
		p := fixture(t, "values/a.xml", `<resources><string name="surrogate">`+source+`</string></resources>`)
		c, e := Load(p, "en")
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(c.Entries[0].SkipReason, "surrogate") || len(c.Warnings) != 1 {
			t.Errorf("surrogate not reported: %#v", c)
		}
	}
}

func TestCarQualifierIsNotALanguage(t *testing.T) {
	folder, err := LocaleFolder("car")
	if err != nil || folder != "values-b+car" {
		t.Fatalf("car language folder: %q %v", folder, err)
	}
	if _, ok := LanguageForFolder("values-car"); ok {
		t.Fatal("car UI-mode qualifier treated as a language")
	}
	p := fixture(t, "values/strings.xml", `<resources><string name="hello">Hello</string></resources>`)
	root := filepath.Dir(filepath.Dir(p))
	carMode := filepath.Join(root, "values-car", "strings.xml")
	original := `<resources><string name="hello">Car mode</string></resources>`
	put(t, carMode, original)
	catalog, err := Load(p, "en")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := catalog.Localizations["car"]; exists {
		t.Fatal("car-mode resources used as language context")
	}
	if err := SaveTranslations(p, "car", []Translation{{Key: "hello", Text: "Car language"}}); err != nil {
		t.Fatal(err)
	}
	if read(t, carMode) != original {
		t.Fatal("modified car-mode resources")
	}
	catalog, err = Load(p, "en")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Localizations["car"]["string/hello"].Text != "Car language" {
		t.Fatal("language resource not saved to BCP 47 folder")
	}
	if folder, err := LocaleFolder("fil"); err != nil || folder != "values-b+fil" {
		t.Fatalf("three-letter language folder: %q %v", folder, err)
	}
}
