package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPathRespectsXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if got, want := Path(), filepath.Join(dir, "xlocal", "config.json"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestLoadMissingFileReturnsEmptySettings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Keys) != 0 || s.DefaultKey != "" {
		t.Errorf("expected zero-value settings, got %+v", s)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s := &Settings{
		Keys:       []string{"private", "work"},
		DefaultKey: "private",
		Model:      "sonnet",
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, s) {
		t.Errorf("round trip mismatch:\ngot  %+v\nwant %+v", loaded, s)
	}

	// The file may hold key names but never secrets; keep it user-only anyway.
	info, err := os.Stat(Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("config file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestAddRemoveKey(t *testing.T) {
	s := &Settings{}

	s.AddKey("private")
	s.AddKey("work")
	s.AddKey("private") // duplicate is a no-op
	if !reflect.DeepEqual(s.Keys, []string{"private", "work"}) {
		t.Errorf("Keys = %v", s.Keys)
	}
	// First key becomes the default automatically.
	if s.DefaultKey != "private" {
		t.Errorf("DefaultKey = %q, want private", s.DefaultKey)
	}

	s.RemoveKey("private")
	if !reflect.DeepEqual(s.Keys, []string{"work"}) {
		t.Errorf("Keys after remove = %v", s.Keys)
	}
	// Default falls back to the remaining key.
	if s.DefaultKey != "work" {
		t.Errorf("DefaultKey after remove = %q, want work", s.DefaultKey)
	}

	s.RemoveKey("work")
	if s.DefaultKey != "" || len(s.Keys) != 0 {
		t.Errorf("expected empty settings, got %+v", s)
	}
}

func TestHasKey(t *testing.T) {
	s := &Settings{Keys: []string{"a"}}
	if !s.HasKey("a") || s.HasKey("b") {
		t.Error("HasKey misbehaves")
	}
}

func TestModelCache(t *testing.T) {
	now := time.Now()
	s := &Settings{}

	if _, ok := s.CachedModel("sonnet", now); ok {
		t.Error("empty cache should miss")
	}

	s.SetCachedModel("sonnet", "claude-sonnet-5", now)

	if id, ok := s.CachedModel("sonnet", now.Add(23*time.Hour)); !ok || id != "claude-sonnet-5" {
		t.Errorf("fresh cache should hit: %q, %v", id, ok)
	}
	if _, ok := s.CachedModel("sonnet", now.Add(25*time.Hour)); ok {
		t.Error("cache older than 24h should miss")
	}
	if _, ok := s.CachedModel("opus", now); ok {
		t.Error("other alias should miss")
	}
}
