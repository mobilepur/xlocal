package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MobilePur/xlocal/internal/android"
)

// FindAndroidSources reads only explicitly configured resource roots. Paths
// belong to the config that declares them, including nested module configs.
func (r *ConfigResolver) FindAndroidSources() ([]string, error) {
	seen := map[string]bool{}
	for configDir, cfg := range r.configs {
		for _, rel := range cfg.AndroidResources {
			if rel == "" || filepath.IsAbs(rel) {
				return nil, fmt.Errorf("androidResources must contain nonempty relative paths: %q", rel)
			}
			root := filepath.Join(configDir, rel)
			inside, err := filepath.Rel(configDir, root)
			if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("androidResources path escapes its config directory: %q", rel)
			}
			if r.catalogExcluded(filepath.Join(root, "values", "strings.xml")) {
				continue
			}
			if info, err := os.Stat(filepath.Join(root, "values")); err != nil || !info.IsDir() {
				return nil, fmt.Errorf("androidResources %s must contain a values directory", root)
			}
			paths, err := android.FindSources(root)
			if err != nil {
				return nil, fmt.Errorf("androidResources %s: %w", root, err)
			}
			for _, path := range paths {
				if !r.catalogExcluded(path) {
					seen[path] = true
				}
			}
		}
	}
	var paths []string
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

// DiscoverAndroidResourceDirs finds standard main source sets for init. Other
// source sets and custom Gradle paths must be selected explicitly in the config.
func DiscoverAndroidResourceDirs(root string) ([]string, error) {
	var roots []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.Name() == "res" && filepath.Base(filepath.Dir(path)) == "main" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "src" {
			paths, err := android.FindSources(path)
			if err != nil {
				return err
			}
			if len(paths) > 0 {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				roots = append(roots, filepath.ToSlash(rel))
			}
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(roots)
	return roots, err
}
