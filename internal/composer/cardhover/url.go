package cardhover

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer/manifest"
)

// ExternalURL returns the canonical external URL for a composer package.
//
// Resolution order:
//
//  1. If the workspace's repositories block declares an override whose URL
//     points at the same package (matched by basename, see below), that
//     URL is returned, normalized.
//  2. Otherwise: https://packagist.org/packages/{name} (no remote lookup
//     needed — Packagist's URL scheme is deterministic).
//
// rootPath is used only to resolve `type: path` repositories to absolute
// file:// URLs. May be empty.
//
// Match heuristic for repositories: a repository URL is considered a
// candidate when its basename, stripped of `.git`, matches the package's
// vendor/name's basename (the "name" half). The workspace composer.json
// rarely has more than a handful of repository entries, so the loose
// match plus the de facto convention (a fork at github.com/your-org/foo
// for package vendor/foo) is good enough. We do not pretend to deeply
// understand `type: composer` private repositories — those fall through
// to Packagist.
func ExternalURL(packageName string, repos []manifest.RepositoryEntry, rootPath string) string {
	if override := matchRepo(packageName, repos, rootPath); override != "" {
		return override
	}
	return "https://packagist.org/packages/" + packageName
}

func matchRepo(packageName string, repos []manifest.RepositoryEntry, rootPath string) string {
	basename := packageBasename(packageName)
	if basename == "" {
		return ""
	}
	for _, r := range repos {
		switch strings.ToLower(r.Type) {
		case "vcs", "git", "github":
			if repoBasename(r.URL) == basename {
				return normalizeVCSURL(r.URL)
			}
		case "path":
			if filepath.Base(strings.TrimRight(r.URL, "/")) == basename {
				return resolvePath(r.URL, rootPath)
			}
		}
	}
	return ""
}

// packageBasename returns the "name" part of a "vendor/name" package id.
// Returns "" when the input has no slash (which would be an invalid
// composer name anyway).
func packageBasename(name string) string {
	idx := strings.LastIndex(name, "/")
	if idx < 0 {
		return ""
	}
	return name[idx+1:]
}

// repoBasename returns the trailing path segment of a git repository URL,
// stripping a trailing .git suffix. Works for both
// "https://host/owner/repo[.git]" and "git@host:owner/repo[.git]".
func repoBasename(repoURL string) string {
	u := strings.TrimSpace(repoURL)
	if u == "" {
		return ""
	}
	if i := strings.LastIndex(u, "/"); i >= 0 {
		u = u[i+1:]
	} else if i := strings.LastIndex(u, ":"); i >= 0 {
		// SCP-style host:path with no slash after host.
		u = u[i+1:]
	}
	return strings.TrimSuffix(u, ".git")
}

// normalizeVCSURL converts SSH-style and .git-suffixed URLs to the
// browsable HTTPS form. Unknown URL shapes pass through unchanged.
func normalizeVCSURL(repoURL string) string {
	u := strings.TrimSpace(repoURL)
	u = strings.TrimSuffix(u, ".git")
	// git@github.com:owner/repo → https://github.com/owner/repo
	if strings.HasPrefix(u, "git@") {
		if i := strings.Index(u, ":"); i >= 0 {
			host := u[len("git@"):i]
			path := strings.TrimPrefix(u[i+1:], "/")
			return "https://" + host + "/" + path
		}
	}
	// Already https/http: pass through.
	return u
}

// resolvePath builds a file:// URL for a `type: path` repository. When
// repoPath is relative, it's resolved against rootPath; when rootPath is
// empty the relative input passes through unchanged.
func resolvePath(repoPath, rootPath string) string {
	p := repoPath
	if !filepath.IsAbs(p) && rootPath != "" {
		p = filepath.Join(rootPath, p)
	}
	abs, err := filepath.Abs(p)
	if err == nil {
		p = abs
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p)}
	return u.String()
}
