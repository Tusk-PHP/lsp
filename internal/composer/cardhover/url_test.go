package cardhover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/composer/manifest"
)

func TestExternalURLDefaultsToPackagist(t *testing.T) {
	got := ExternalURL("laravel/framework", nil, "")
	want := "https://packagist.org/packages/laravel/framework"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExternalURLPicksRepoOverride(t *testing.T) {
	repos := []manifest.RepositoryEntry{
		{Type: "vcs", URL: "https://github.com/acme/framework.git"},
	}
	got := ExternalURL("acme/framework", repos, "")
	want := "https://github.com/acme/framework"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExternalURLNormalizesSSHGit(t *testing.T) {
	repos := []manifest.RepositoryEntry{
		{Type: "git", URL: "git@github.com:acme/fork.git"},
	}
	got := ExternalURL("acme/fork", repos, "")
	want := "https://github.com/acme/fork"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExternalURLBasenameMismatchFallsBackToPackagist(t *testing.T) {
	// repository URL points at a different package — should NOT match.
	repos := []manifest.RepositoryEntry{
		{Type: "vcs", URL: "https://github.com/acme/something-else.git"},
	}
	got := ExternalURL("acme/widgets", repos, "")
	want := "https://packagist.org/packages/acme/widgets"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExternalURLPathRepository(t *testing.T) {
	repos := []manifest.RepositoryEntry{
		{Type: "path", URL: "../local-fork"},
	}
	got := ExternalURL("acme/local-fork", repos, "/tmp/project")
	if !strings.HasPrefix(got, "file:///") {
		t.Errorf("expected file:// URL, got %q", got)
	}
	if !strings.HasSuffix(got, "/local-fork") {
		t.Errorf("expected URL ending with /local-fork, got %q", got)
	}
}

func TestExternalURLInvalidPackageNameStillReturnsPackagistURL(t *testing.T) {
	// Pathological input: no slash. We still return *something* —
	// downstream rendering handles empty/garbage gracefully.
	got := ExternalURL("notapackage", nil, "")
	want := "https://packagist.org/packages/notapackage"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
