// Package settings manages the per-user configuration in
// ~/.config/xlocal/config.json: names of stored API keys (the secrets
// themselves live in the keychain), the default key, the preferred model,
// and a small cache for resolved model IDs.
package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type Settings struct {
	Keys       []string                   `json:"keys,omitempty"`
	DefaultKey string                     `json:"defaultKey,omitempty"`
	Model      string                     `json:"model,omitempty"`
	ModelCache map[string]ModelCacheEntry `json:"modelCache,omitempty"`
}

type ModelCacheEntry struct {
	ID         string    `json:"id"`
	ResolvedAt time.Time `json:"resolvedAt"`
}

// modelCacheTTL bounds how long a resolved model alias (e.g. "sonnet" →
// "claude-sonnet-x") is reused before asking the API again.
const modelCacheTTL = 24 * time.Hour

// Path returns the settings file location, honoring XDG_CONFIG_HOME.
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "xlocal", "config.json")
}

// Load reads the settings file; a missing file yields empty settings.
func Load() (*Settings, error) {
	data, err := os.ReadFile(Path())
	if errors.Is(err, fs.ErrNotExist) {
		return &Settings{}, nil
	}
	if err != nil {
		return nil, err
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Settings) Save() error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func (s *Settings) HasKey(name string) bool {
	for _, k := range s.Keys {
		if k == name {
			return true
		}
	}
	return false
}

// AddKey registers a key name; the first key becomes the default.
func (s *Settings) AddKey(name string) {
	if s.HasKey(name) {
		return
	}
	s.Keys = append(s.Keys, name)
	if s.DefaultKey == "" {
		s.DefaultKey = name
	}
}

// RemoveKey unregisters a key name and keeps the default valid.
func (s *Settings) RemoveKey(name string) {
	kept := s.Keys[:0]
	for _, k := range s.Keys {
		if k != name {
			kept = append(kept, k)
		}
	}
	s.Keys = kept
	if len(s.Keys) == 0 {
		s.Keys = nil
	}

	if s.DefaultKey == name {
		s.DefaultKey = ""
		if len(s.Keys) > 0 {
			s.DefaultKey = s.Keys[0]
		}
	}
}

// CachedModel returns the cached model ID for an alias if it is still fresh.
func (s *Settings) CachedModel(alias string, now time.Time) (string, bool) {
	entry, ok := s.ModelCache[alias]
	if !ok || now.Sub(entry.ResolvedAt) > modelCacheTTL {
		return "", false
	}
	return entry.ID, true
}

func (s *Settings) SetCachedModel(alias, id string, now time.Time) {
	if s.ModelCache == nil {
		s.ModelCache = make(map[string]ModelCacheEntry)
	}
	s.ModelCache[alias] = ModelCacheEntry{ID: id, ResolvedAt: now}
}
