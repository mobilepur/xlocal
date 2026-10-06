package project

import (
	"os"
	"path/filepath"
	"testing"
)

func androidRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func writeAndroidConfig(t *testing.T, root, body string) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func writeAndroidXML(t *testing.T, root, rel string) {
	t.Helper()
	path := filepath.Join(root, rel, "values", "texts.xml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`<resources><string name="hello">Hello</string></resources>`), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestFindAndroidSourcesUsesDeclaringConfigAndExclusions(t *testing.T) {
	root := androidRoot(t)
	writeAndroidConfig(t, root, `{"targetLanguages":["de"],"androidResources":["app/src/main/res","ignored/src/main/res"],"exclude":["ignored"]}`)
	module := filepath.Join(root, "feature")
	writeAndroidConfig(t, module, `{"androidResources":["src/main/res"],"androidSourceLanguage":"fr"}`)
	writeAndroidXML(t, root, "app/src/main/res")
	writeAndroidXML(t, root, "ignored/src/main/res")
	writeAndroidXML(t, module, "src/main/res")
	resolver, err := NewConfigResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := resolver.FindAndroidSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != filepath.Join(root, "app/src/main/res/values/texts.xml") || paths[1] != filepath.Join(module, "src/main/res/values/texts.xml") {
		t.Fatalf("paths: %v", paths)
	}
	if cfg := resolver.Resolve(filepath.Dir(paths[1])); cfg.AndroidSourceLanguage != "fr" || len(cfg.TargetLanguages) != 1 || cfg.TargetLanguages[0] != "de" {
		t.Fatalf("module config: %+v", cfg)
	}
}
func TestFindAndroidSourcesRejectsEscapingAndMissingRoots(t *testing.T) {
	for _, resource := range []string{"../outside", "/private/tmp/outside", "missing", ""} {
		t.Run(resource, func(t *testing.T) {
			root := androidRoot(t)
			writeAndroidConfig(t, root, `{"targetLanguages":["de"],"androidResources":["`+resource+`"]}`)
			resolver, err := NewConfigResolver(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := resolver.FindAndroidSources(); err == nil {
				t.Fatal("accepted invalid resource root")
			}
		})
	}
}
func TestDiscoverAndroidMainResourcesOnly(t *testing.T) {
	root := androidRoot(t)
	writeAndroidXML(t, root, "app/src/main/res")
	writeAndroidXML(t, root, "app/src/debug/res")
	writeAndroidXML(t, root, "build/generated/src/main/res")
	dirs, err := DiscoverAndroidResourceDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != "app/src/main/res" {
		t.Fatalf("dirs: %v", dirs)
	}
}
