// Package project locates Xcode projects, loads the per-project
// xlocal-config.json, and finds String Catalogs within a project.
package project

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ConfigFileName is the per-project configuration file, checked into the
// Xcode project's repository. It never contains secrets.
const ConfigFileName = "xlocal-config.json"

type Config struct {
	TargetLanguages     []string `json:"targetLanguages"`
	BaseLanguages       []string `json:"baseLanguages,omitempty"`
	UntranslatableWords []string `json:"untranslatableWords,omitempty"`
	FormalLanguages     []string `json:"formalLanguages,omitempty"`
	Model               string   `json:"model,omitempty"`
	Exclude             []string `json:"exclude,omitempty"`
	CustomPrompt        string   `json:"customPrompt,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s: invalid config: %w", path, err)
	}
	if len(cfg.TargetLanguages) == 0 {
		return nil, fmt.Errorf("%s: targetLanguages must not be empty", path)
	}

	return &cfg, nil
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// FindConfigUpwards walks from startDir towards the filesystem root and
// returns the path of the first xlocal-config.json it finds.
func FindConfigUpwards(startDir string) (string, bool) {
	dir := startDir
	for {
		candidate := filepath.Join(dir, ConfigFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// skipDirs are directory names that never contain user-facing catalogs and
// would only slow down (or pollute) discovery.
var skipDirs = map[string]bool{
	".git":         true,
	".build":       true,
	"build":        true,
	"DerivedData":  true,
	"Pods":         true,
	"Carthage":     true,
	"node_modules": true,
	".swiftpm":     true,
}

// Candidate is a directory that looks like an Xcode project.
type Candidate struct {
	Dir       string
	HasConfig bool
	Kind      string // "config", "xcodeproj", "xcworkspace" or "package"
}

// DiscoverProjects scans downward from root (up to maxDepth directory levels)
// for directories containing an xlocal config, an .xcodeproj/.xcworkspace or
// a Package.swift. Candidates with a config are listed first.
func DiscoverProjects(root string, maxDepth int) ([]Candidate, error) {
	byDir := map[string]*Candidate{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}
		depth := len(strings.Split(filepath.ToSlash(rel), "/"))

		if d.IsDir() {
			name := d.Name()
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if depth > maxDepth {
				return filepath.SkipDir
			}

			switch {
			case strings.HasSuffix(name, ".xcodeproj"):
				markCandidate(byDir, filepath.Dir(path), "xcodeproj", false)
				return filepath.SkipDir // don't descend into the bundle
			case strings.HasSuffix(name, ".xcworkspace"):
				markCandidate(byDir, filepath.Dir(path), "xcworkspace", false)
				return filepath.SkipDir
			}
			return nil
		}

		switch d.Name() {
		case ConfigFileName:
			markCandidate(byDir, filepath.Dir(path), "config", true)
		case "Package.swift":
			markCandidate(byDir, filepath.Dir(path), "package", false)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	candidates := make([]Candidate, 0, len(byDir))
	for _, c := range byDir {
		candidates = append(candidates, *c)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].HasConfig != candidates[j].HasConfig {
			return candidates[i].HasConfig
		}
		return candidates[i].Dir < candidates[j].Dir
	})

	return candidates, nil
}

func markCandidate(byDir map[string]*Candidate, dir, kind string, hasConfig bool) {
	if existing, ok := byDir[dir]; ok {
		if hasConfig {
			existing.HasConfig = true
			existing.Kind = kind
		}
		return
	}
	byDir[dir] = &Candidate{Dir: dir, HasConfig: hasConfig, Kind: kind}
}

// FindCatalogs returns all .xcstrings files below root, sorted by path.
// Default build/dependency directories are always skipped; exclude adds
// project-specific directory names or root-relative paths.
func FindCatalogs(root string, exclude []string) ([]string, error) {
	var catalogs []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." {
			return nil
		}

		if d.IsDir() {
			name := d.Name()
			if skipDirs[name] || strings.HasPrefix(name, ".") || isExcluded(rel, name, exclude) {
				return filepath.SkipDir
			}
			if strings.HasSuffix(name, ".xcodeproj") || strings.HasSuffix(name, ".xcworkspace") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(d.Name(), ".xcstrings") {
			catalogs = append(catalogs, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(catalogs)
	return catalogs, nil
}

// isExcluded reports whether a directory matches an exclude entry, either by
// bare name ("Pods") or by root-relative path prefix ("App/Generated").
func isExcluded(rel, name string, exclude []string) bool {
	relSlash := filepath.ToSlash(rel)
	for _, e := range exclude {
		e = strings.TrimSuffix(filepath.ToSlash(e), "/")
		if e == "" {
			continue
		}
		if name == e || relSlash == e || strings.HasPrefix(relSlash, e+"/") {
			return true
		}
	}
	return false
}
