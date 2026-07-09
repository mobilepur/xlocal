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
	ExcludeKeys         []string `json:"excludeKeys,omitempty"`
	CustomPrompt        string   `json:"customPrompt,omitempty"`
}

// LoadConfigRaw reads a config without requiring any field to be set. Nested
// configs are allowed to be partial: they inherit everything they don't set
// from the configs above them (see ConfigResolver).
func LoadConfigRaw(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s: invalid config: %w", path, err)
	}
	return &cfg, nil
}

// LoadConfig reads a config that must stand on its own — targetLanguages is
// required. Use LoadConfigRaw for nested configs that inherit their fields.
func LoadConfig(path string) (*Config, error) {
	cfg, err := LoadConfigRaw(path)
	if err != nil {
		return nil, err
	}
	if len(cfg.TargetLanguages) == 0 {
		return nil, fmt.Errorf("%s: targetLanguages must not be empty", path)
	}
	return cfg, nil
}

// clone returns a shallow copy of a config. Fields are only ever reassigned
// (never mutated in place), so sharing slice backing arrays is safe.
func (c *Config) clone() *Config {
	cp := *c
	return &cp
}

// Merge layers override on top of base and returns the effective config.
//
// Most fields override when the child sets them (a nil slice or empty string
// means "not set, inherit"). untranslatableWords and excludeKeys accumulate as
// a deduplicated union so global brand names keep applying while a subfolder
// adds its own. Model is intentionally NOT taken from override — it stays
// global (the root value), so a whole run uses a single model and client.
func Merge(base, override *Config) *Config {
	out := base.clone()
	if override.TargetLanguages != nil {
		out.TargetLanguages = override.TargetLanguages
	}
	if override.BaseLanguages != nil {
		out.BaseLanguages = override.BaseLanguages
	}
	if override.FormalLanguages != nil {
		out.FormalLanguages = override.FormalLanguages
	}
	if override.CustomPrompt != "" {
		out.CustomPrompt = override.CustomPrompt
	}
	out.UntranslatableWords = unionStrings(base.UntranslatableWords, override.UntranslatableWords)
	out.ExcludeKeys = unionStrings(base.ExcludeKeys, override.ExcludeKeys)
	return out
}

// unionStrings concatenates a and b, preserving order and dropping duplicates.
func unionStrings(a, b []string) []string {
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
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

// FindTopmostConfig walks from startDir towards the filesystem root and
// returns the path of the highest ancestor config it finds — the one closest
// to the root. That directory anchors the project, so nested configs below it
// refine it consistently no matter which subfolder xlocal is invoked from.
func FindTopmostConfig(startDir string) (string, bool) {
	dir := startDir
	topmost := ""
	for {
		candidate := filepath.Join(dir, ConfigFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			topmost = candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if topmost == "" {
		return "", false
	}
	return topmost, true
}

// DiscoverConfigs reads every xlocal-config.json at or below root, keyed by the
// directory that contains it. Configs are loaded leniently (LoadConfigRaw) so
// nested ones may be partial. The user's own exclude list is deliberately not
// applied here — configs must be found before their exclude can take effect.
func DiscoverConfigs(root string) (map[string]*Config, error) {
	configs := map[string]*Config{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil || rel == "." {
				return nil
			}
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if strings.HasSuffix(name, ".xcodeproj") || strings.HasSuffix(name, ".xcworkspace") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ConfigFileName {
			cfg, err := LoadConfigRaw(path)
			if err != nil {
				return err
			}
			configs[filepath.Dir(path)] = cfg
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return configs, nil
}

// ConfigResolver computes the effective config for any directory in a project
// by merging the chain of configs from the project root down to that directory.
type ConfigResolver struct {
	root    string
	configs map[string]*Config // directory -> raw config
	cache   map[string]*Config // directory -> resolved config
}

// NewConfigResolver discovers all configs at or below root and prepares the
// resolver. root is expected to contain a config (it anchors the project); an
// empty base is substituted otherwise so resolution never panics.
func NewConfigResolver(root string) (*ConfigResolver, error) {
	configs, err := DiscoverConfigs(root)
	if err != nil {
		return nil, err
	}
	if _, ok := configs[root]; !ok {
		configs[root] = &Config{}
	}
	return &ConfigResolver{root: root, configs: configs, cache: map[string]*Config{}}, nil
}

// Resolve returns the effective config for dir: the root config with every
// nested config between root and dir layered on top (shallow to deep).
func (r *ConfigResolver) Resolve(dir string) *Config {
	if c, ok := r.cache[dir]; ok {
		return c
	}

	// Collect the config-bearing directories from dir up to root (deep first).
	var chain []string
	d := dir
	for {
		if _, ok := r.configs[d]; ok {
			chain = append(chain, d)
		}
		if d == r.root {
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}

	result := r.configs[r.root].clone()
	// Apply from shallow (just below root) to deep, skipping root itself.
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i] == r.root {
			continue
		}
		result = Merge(result, r.configs[chain[i]])
	}

	r.cache[dir] = result
	return result
}

// Root returns the effective config at the project root — used for settings
// that stay global for a run (model, base-language batch).
func (r *ConfigResolver) Root() *Config {
	return r.Resolve(r.root)
}

// FindCatalogs discovers catalogs like the package-level function, but honours
// each config's exclude within the subtree that declares it: an exclude entry
// in a subfolder's config only skips directories below that subfolder.
func (r *ConfigResolver) FindCatalogs() ([]string, error) {
	return findCatalogs(r.root, func(pathAbs, name, _ string) bool {
		for dir, cfg := range r.configs {
			if len(cfg.Exclude) == 0 {
				continue
			}
			if pathAbs != dir && !strings.HasPrefix(pathAbs, dir+string(filepath.Separator)) {
				continue // rule only applies inside the declaring subtree
			}
			sub, err := filepath.Rel(dir, pathAbs)
			if err != nil {
				continue
			}
			if isExcluded(sub, name, cfg.Exclude) {
				return true
			}
		}
		return false
	})
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
	return findCatalogs(root, func(_, name, rel string) bool {
		return isExcluded(rel, name, exclude)
	})
}

// findCatalogs walks root for .xcstrings files, always skipping the default
// build/dependency directories and .xcodeproj/.xcworkspace bundles. skipExtra,
// if non-nil, decides additional directories to skip; it receives the absolute
// path, the base name, and the root-relative path of each directory.
func findCatalogs(root string, skipExtra func(pathAbs, name, rel string) bool) ([]string, error) {
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
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if strings.HasSuffix(name, ".xcodeproj") || strings.HasSuffix(name, ".xcworkspace") {
				return filepath.SkipDir
			}
			if skipExtra != nil && skipExtra(path, name, rel) {
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
