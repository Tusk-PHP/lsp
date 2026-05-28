// Package cardhover implements LSP hover and definition for composer.json
// documents. It surfaces package metadata (name, PHP requirement,
// description, license, source URL) by layering composer.lock as the
// primary fast-path source, then falling back to the in-document
// constraint when the package isn't locked.
package cardhover

import (
	"path/filepath"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer/lockfile"
	"github.com/Tusk-PHP/lsp/internal/composer/manifest"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// Provider serves textDocument/hover and textDocument/definition for
// composer.json files. It does not own any data — lockfile lookups happen
// fresh on each hover so edits to composer.lock are picked up without
// invalidation.
type Provider struct {
	rootPath        string
	projectPHPVer   string                                 // "MAJOR.MINOR" of the workspace platform
	loadLockfile    func(root string) (*lockfile.Lockfile, error) // injectable for tests
	enabled         bool
	openOnDefinition bool
}

// NewProvider constructs a Provider bound to a workspace root. projectPHPVer
// is the resolved platform PHP version used to render the compatibility
// glyph in the hover card; empty disables the glyph.
func NewProvider(rootPath, projectPHPVer string) *Provider {
	return &Provider{
		rootPath:      rootPath,
		projectPHPVer: projectPHPVer,
		loadLockfile:  lockfile.Load,
		enabled:       true,
	}
}

// SetEnabled toggles the whole feature. When false, Hover and Definition
// return nil unconditionally.
func (p *Provider) SetEnabled(v bool) { p.enabled = v }

// SetOpenOnDefinition controls whether Definition reports an external URL
// for the cursor's package. Off by default.
func (p *Provider) SetOpenOnDefinition(v bool) { p.openOnDefinition = v }

// IsComposerJSON reports whether the given URI/path refers to a
// composer.json file the provider should answer for. Vendor composer.json
// files are accepted — they get the same treatment as the workspace's own
// file, per the design (see .claude/plans/composer-hover.md §1).
func IsComposerJSON(uri string) bool {
	path := uriPath(uri)
	return filepath.Base(path) == "composer.json"
}

// Hover renders the package card for the cursor's position, or nil when
// the cursor isn't on a require/require-dev entry.
func (p *Provider) Hover(uri, source string, pos protocol.Position) *protocol.Hover {
	if p == nil || !p.enabled {
		return nil
	}
	entry, repos := p.entryAt(source, pos)
	if entry == nil {
		return nil
	}
	card := p.buildCard(*entry, repos)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: "markdown", Value: card.render()},
		Range:    &entry.NameRange,
	}
}

// DefinitionResult is what the server uses to decide whether to dispatch
// a window/showDocument request. ExternalURL is empty when the cursor
// isn't on a known package or when openOnDefinition is disabled.
type DefinitionResult struct {
	ExternalURL string
}

// Definition reports the external URL for the cursor's package, gated by
// SetOpenOnDefinition. Empty result means the server should respond with
// a null Location.
func (p *Provider) Definition(uri, source string, pos protocol.Position) DefinitionResult {
	if p == nil || !p.enabled || !p.openOnDefinition {
		return DefinitionResult{}
	}
	entry, repos := p.entryAt(source, pos)
	if entry == nil {
		return DefinitionResult{}
	}
	return DefinitionResult{ExternalURL: ExternalURL(entry.Name, repos, p.rootPath)}
}

// entryAt parses the document and locates the require entry under the
// cursor, returning both it and the document's repositories block for
// URL resolution.
func (p *Provider) entryAt(source string, pos protocol.Position) (*manifest.RequireEntry, []manifest.RepositoryEntry) {
	m := manifest.Parse(source)
	entry := m.PackageAtCursor(pos)
	if entry == nil {
		return nil, nil
	}
	// "php" / "ext-*" entries in require are platform constraints, not
	// packages. They have no Packagist page and the card would be
	// confusing — skip them.
	if isPlatformConstraint(entry.Name) {
		return nil, nil
	}
	return entry, m.Repositories
}

func isPlatformConstraint(name string) bool {
	if name == "php" || name == "hhvm" {
		return true
	}
	return strings.HasPrefix(name, "ext-") || strings.HasPrefix(name, "lib-") || strings.HasPrefix(name, "php-")
}

// buildCard assembles the data shown in the hover, layering sources in
// the precedence order defined by the plan: lockfile (when available),
// then the in-document constraint as the always-present fallback.
func (p *Provider) buildCard(entry manifest.RequireEntry, repos []manifest.RepositoryEntry) card {
	c := card{
		Name:            entry.Name,
		Constraint:      entry.Value,
		ExternalURL:     ExternalURL(entry.Name, repos, p.rootPath),
		ProjectPHPVer:   p.projectPHPVer,
	}
	// Locked data, if available.
	if lock, err := p.loadLockfile(p.rootPath); err == nil && lock != nil {
		if pkg, ok := lock.Find(entry.Name); ok {
			c.Description = pkg.Description
			c.InstalledVersion = pkg.Version
			c.PHPRequire = pkg.PHPRequire
			c.License = pkg.License
			c.Homepage = pkg.Homepage
			c.SourceURL = pkg.SourceURL
		}
	}
	return c
}

// uriPath converts a "file://..." URI to a filesystem path. We accept a
// plain path too, so tests don't need to URI-encode.
func uriPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}
