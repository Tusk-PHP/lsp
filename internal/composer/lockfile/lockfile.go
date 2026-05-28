// Package lockfile reads composer.lock and exposes per-package metadata
// needed by the composer.json hover provider. It is the fast-path source —
// no network, no shell-out, just JSON parsing.
package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LockedPackage is the subset of composer.lock package data the hover
// provider consumes.
type LockedPackage struct {
	Name        string
	Version     string   // exact resolved version, e.g. "v10.48.4"
	Description string
	PHPRequire  string   // raw constraint from require.php, e.g. ">=8.1"
	License     []string // licenses, e.g. ["MIT"]
	Homepage    string
	SourceURL   string // source.url
	SourceType  string // source.type (e.g. "git")
}

// Lockfile is the parsed view of one composer.lock.
type Lockfile struct {
	byName map[string]LockedPackage
}

// Find returns the locked package by name, or ok=false when absent.
func (l *Lockfile) Find(name string) (LockedPackage, bool) {
	if l == nil {
		return LockedPackage{}, false
	}
	pkg, ok := l.byName[name]
	return pkg, ok
}

// Load reads composer.lock from the given workspace root. Returns nil when
// the file does not exist; an error only on read or parse failure.
func Load(rootPath string) (*Lockfile, error) {
	path := filepath.Join(rootPath, "composer.lock")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var raw struct {
		Packages    []rawPackage `json:"packages"`
		PackagesDev []rawPackage `json:"packages-dev"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	l := &Lockfile{byName: make(map[string]LockedPackage, len(raw.Packages)+len(raw.PackagesDev))}
	for _, p := range raw.Packages {
		l.byName[p.Name] = toLocked(p)
	}
	for _, p := range raw.PackagesDev {
		l.byName[p.Name] = toLocked(p)
	}
	return l, nil
}

type rawPackage struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	License     []string          `json:"license"`
	Homepage    string            `json:"homepage"`
	Require     map[string]string `json:"require"`
	Source      struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"source"`
}

func toLocked(p rawPackage) LockedPackage {
	return LockedPackage{
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		PHPRequire:  p.Require["php"],
		License:     p.License,
		Homepage:    p.Homepage,
		SourceURL:   p.Source.URL,
		SourceType:  p.Source.Type,
	}
}
