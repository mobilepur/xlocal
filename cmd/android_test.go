package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MobilePur/xlocal/internal/analyze"
	"github.com/MobilePur/xlocal/internal/anthropic"
	"github.com/MobilePur/xlocal/internal/translate"
)

func androidProject(t *testing.T) (string, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeTestConfig(t, root, `{"targetLanguages":["en","de"],"androidResources":["app/src/main/res"],"androidSourceLanguage":"en"}`)
	res := filepath.Join(root, "app", "src", "main", "res")
	for dir, text := range map[string]string{
		"values": `<resources>
<!-- Greeting on the home screen -->
<string name="welcome">Hello, %1$s!</string>
<plurals name="files"><item quantity="one">%1$d file</item><item quantity="other">%1$d files</item></plurals>
<string name="brand" translatable="false">xlocal</string>
<string name="styled">A <b>bold</b> word</string>
<string-array name="options"><item>First</item></string-array>
</resources>`,
		"values-de": `<resources>
<!-- This translation was reviewed by a human. -->
<plurals name="files"><item quantity="one">%1$d Datei</item></plurals>
<color name="accent">#112233</color>
</resources>`,
	} {
		path := filepath.Join(res, dir, "strings.xml")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root, res
}

func TestAndroidAnalyzeTranslateAndSave(t *testing.T) {
	root, res := androidProject(t)
	pc, err := newProjectContext(root)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := analyzeProject(pc)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || len(reports[0].Missing) != 2 {
		t.Fatalf("reports: %+v", reports)
	}
	var batch []analyze.Missing
	for _, m := range reports[0].Missing {
		if m.TargetLanguage != "de" {
			t.Fatalf("source language treated as missing: %+v", m)
		}
		batch = append(batch, m)
	}
	source := filepath.Join(res, "values", "strings.xml")
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body.Messages) == 0 {
			t.Error("missing prompt")
			http.Error(w, "no prompt", 400)
			return
		}
		prompt := body.Messages[0].Content
		if !strings.Contains(prompt, "Android") {
			t.Error("missing Android context")
		}
		response := "Hallo, %1$s!"
		if strings.Contains(prompt, "PLURAL FORM REQUIRED") {
			response = "one: %1$d NEUE Datei | other: %1$d Dateien"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []map[string]string{{"type": "text", "text": response}}})
	}))
	defer server.Close()
	client := anthropic.New("test-key", "test-model")
	client.BaseURL = server.URL
	results := runTranslations(context.Background(), func(string) *anthropic.Client { return client }, batch, func(string) translate.Options { return translate.Options{} })
	ok, failed := splitResults(results)
	if len(ok) != 2 || len(failed) != 0 {
		t.Fatalf("results: %+v", results)
	}
	if err := writeResults(ok, root); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(source)
	if string(before) != string(after) {
		t.Error("modified source resources")
	}
	data, _ := os.ReadFile(filepath.Join(res, "values-de", "strings.xml"))
	for _, want := range []string{"%1$d Datei", "%1$d Dateien", "Hallo, %1$s!", "#112233", "reviewed by a human"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing preserved/generated content %q: %s", want, data)
		}
	}
	if strings.Contains(string(data), "NEUE") {
		t.Error("replaced existing plural form")
	}
	reports, err = analyzeProject(pc)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || len(reports[0].Missing) != 0 {
		t.Fatalf("translations still missing: %+v", reports)
	}
	if aapt, jar := os.Getenv("XLOCAL_AAPT2"), os.Getenv("XLOCAL_ANDROID_JAR"); aapt != "" && jar != "" {
		out := t.TempDir()
		compiled := filepath.Join(out, "resources.zip")
		command := exec.Command(aapt, "compile", "--dir", res, "-o", compiled)
		if data, err := command.CombinedOutput(); err != nil {
			t.Fatalf("AAPT2 compile pipeline output: %v\n%s", err, data)
		}
		manifest := filepath.Join(out, "AndroidManifest.xml")
		if err := os.WriteFile(manifest, []byte(`<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.mobilepur.xlocal.test"><application android:label="xlocal"/></manifest>`), 0644); err != nil {
			t.Fatal(err)
		}
		command = exec.Command(aapt, "link", "-I", jar, "--manifest", manifest, "-o", filepath.Join(out, "resources.apk"), compiled)
		if data, err := command.CombinedOutput(); err != nil {
			t.Fatalf("AAPT2 link pipeline output: %v\n%s", err, data)
		}
		t.Log("generated translation pipeline output compiled and linked with AAPT2")
	}

}

func TestAndroidRejectsInvalidPlainFormatBeforeSaving(t *testing.T) {
	root, _ := androidProject(t)
	pc, err := newProjectContext(root)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := analyzeProject(pc)
	if err != nil {
		t.Fatal(err)
	}
	var batch []analyze.Missing
	for _, m := range reports[0].Missing {
		if m.Key == "welcome" {
			batch = append(batch, m)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []map[string]string{{"type": "text", "text": "Hallo, %1$d!"}}})
	}))
	defer server.Close()
	client := anthropic.New("test", "test")
	client.BaseURL = server.URL
	results := runTranslations(context.Background(), func(string) *anthropic.Client { return client }, batch, func(string) translate.Options { return translate.Options{} })
	ok, failed := splitResults(results)
	if len(ok) != 0 || len(failed) != 1 {
		t.Fatalf("unsafe Android translation allowed: %+v", results)
	}
}

func TestAndroidInitDetectsResourcesAndLanguages(t *testing.T) {
	root, _ := androidProject(t)
	if err := os.Remove(filepath.Join(root, "xlocal-config.json")); err != nil {
		t.Fatal(err)
	}
	dir, err := resolveInitDir(root)
	if err != nil || dir != root {
		t.Fatalf("init dir: %q %v", dir, err)
	}
	cfg, err := createConfigSkeleton(root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "xlocal-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "app/src/main/res") || !strings.Contains(string(data), "androidSourceLanguage") {
		t.Fatalf("missing Android configuration: %s", data)
	}
	if len(cfg.TargetLanguages) != 2 {
		t.Fatalf("detected languages: %+v", cfg)
	}
	pc, err := newProjectContext(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analyzeProject(pc); err != nil {
		t.Fatal(err)
	}
}

func TestAndroidSourceChangesRejectStaleTranslations(t *testing.T) {
	root, res := androidProject(t)
	pc, err := newProjectContext(root)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := analyzeProject(pc)
	if err != nil {
		t.Fatal(err)
	}
	var m analyze.Missing
	for _, missing := range reports[0].Missing {
		if missing.Key == "welcome" {
			m = missing
		}
	}
	source := filepath.Join(res, "values", "strings.xml")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(strings.Replace(string(data), "Hello, %1$s!", "Welcome, %1$s!", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(res, "values-de", "strings.xml")
	before, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeResults([]translate.Result{{Missing: m, Translation: "Hallo, %1$s!"}}, root); err == nil {
		t.Fatal("saved stale translation")
	}
	after, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("modified destination despite stale source")
	}
}
func TestAndroidCanonicalLocales(t *testing.T) {
	root, _ := androidProject(t)
	writeTestConfig(t, root, `{"targetLanguages":["EN","DE","de"],"androidResources":["app/src/main/res"],"androidSourceLanguage":"EN"}`)
	pc, err := newProjectContext(root)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := analyzeProject(pc)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports[0].Missing) != 2 {
		t.Fatalf("duplicate or source locale missing: %+v", reports[0])
	}
	for _, m := range reports[0].Missing {
		if m.SourceLanguage != "en" || m.TargetLanguage != "de" {
			t.Fatalf("uncanonicalized locale: %+v", m)
		}
	}
}

func TestAndroidSkipsUnwritableDestinationBeforeAPI(t *testing.T) {
	root, res := androidProject(t)
	source := filepath.Join(res, "values", "strings.xml")
	if err := os.WriteFile(source, []byte(`<resources><string name="literal" formatted="false">100% ready</string><string name="locked">Locked</string><string name="ok">OK</string></resources>`), 0644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(res, "values-de", "strings.xml")
	if err := os.WriteFile(target, []byte(`<resources><string name="literal"/><string name="locked" translatable="false"/></resources>`), 0644); err != nil {
		t.Fatal(err)
	}
	pc, err := newProjectContext(root)
	if err != nil {
		t.Fatal(err)
	}
	reports, err := analyzeProject(pc)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || len(reports[0].Missing) != 1 || reports[0].Missing[0].Key != "ok" {
		t.Fatalf("unwritable resource offered for translation: %+v", reports[0].Missing)
	}
	if len(reports[0].Warnings) != 2 {
		t.Fatalf("unwritable resources not reported: %+v", reports[0].Warnings)
	}
}
