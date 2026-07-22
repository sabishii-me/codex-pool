package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadProviderSpecsDir loads a complete declarative snapshot from JSON files.
// Ordering is deterministic and any invalid file rejects the whole snapshot.
func LoadProviderSpecsDir(dir string) ([]ProviderSpec, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read provider specs dir %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	specs := make([]ProviderSpec, 0, len(names))
	seen := make(map[ProviderID]string, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read provider spec %s: %w", path, err)
		}
		spec, err := ParseProviderSpec(data)
		if err != nil {
			return nil, fmt.Errorf("provider spec %s: %w", path, err)
		}
		if previous, exists := seen[spec.ID]; exists {
			return nil, fmt.Errorf("duplicate provider %q in %s and %s", spec.ID, previous, path)
		}
		seen[spec.ID] = path
		specs = append(specs, spec)
	}
	return specs, nil
}

func ReloadProviderSpecs(registry *ProviderRegistry, dir string) error {
	specs, err := LoadProviderSpecsDir(dir)
	if err != nil {
		return err
	}
	return registry.ReplaceDeclarative(specs)
}
