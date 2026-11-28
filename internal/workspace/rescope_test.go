package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
)

func excluding(paths ...string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.ExcludePaths = paths
	return cfg
}

func TestRescopeKeepsFilesUnderAComposerPackageDirectory(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{
		"vendor/acme/lib/src/Client.php": "<?php\nnamespace Acme;\nclass Client {}\n",
		"vendor/composer/installed.json": `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}}}]}`,
	})
	client := "file://" + filepath.Join(root, "vendor/acme/lib/src/Client.php")
	refreshWith(t, ws, excluding("legacy"), root)
	if !ws.Index.HasFile(client) {
		t.Fatal("a vendor directory no longer excluded is indexed")
	}
	out := refreshWith(t, ws, excluding("vendor"), root)
	if !ws.Index.HasFile(client) || slices.Contains(out.Files, client) {
		t.Errorf("a file under a Composer package's PSR-4 directory stays indexed when vendor is excluded again: %v", out.Files)
	}
}

func TestRescopeSkipsAFileThatIsGoneWhenItIsRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links needs privileges on Windows")
	}
	ws, root := newRefreshWorkspace(t, excluding("vendor", "legacy"), map[string]string{
		"legacy/Old.php": "<?php\nclass OldLegacy {}\n",
	})
	if err := os.Symlink(filepath.Join(root, "missing.php"), filepath.Join(root, "legacy/Gone.php")); err != nil {
		t.Fatal(err)
	}
	out := refreshWith(t, ws, excluding("vendor"), root)
	if !slices.Equal(out.Files, []string{"file://" + filepath.Join(root, "legacy/Old.php")}) {
		t.Errorf("only the file that can be read is indexed, and the one that is gone is no error: %v", out.Files)
	}
}

func TestRescopeReportsAFileItCannotReadAndIndexesTheRest(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions don't stop this user from reading")
	}
	ws, root := newRefreshWorkspace(t, excluding("vendor", "legacy"), map[string]string{
		"legacy/Old.php":    "<?php\nclass OldLegacy {}\n",
		"legacy/Locked.php": "<?php\nclass LockedLegacy {}\n",
	})
	locked := filepath.Join(root, "legacy/Locked.php")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })

	out, err := ws.Refresh(Refresh{Profile: true, Config: settingsOf(excluding("vendor"))})
	if err == nil || !strings.Contains(err.Error(), "index "+locked) {
		t.Errorf("the file that can't be read is reported: %v", err)
	}
	if !slices.Equal(out.Failed, []string{locked}) {
		t.Errorf("the file that can't be read is listed as failed: %v", out.Failed)
	}
	if ws.Index.Lookup("OldLegacy") == nil || ws.Index.Lookup("LockedLegacy") != nil {
		t.Error("the readable file of the newly included directory is indexed, the unreadable one isn't")
	}
	if ws.Config().ExcludePaths[0] != "vendor" || len(ws.Config().ExcludePaths) != 1 {
		t.Errorf("the new settings apply despite the error: %v", ws.Config().ExcludePaths)
	}
}

func TestRescopeErrorKeepsThePHPVersionOfTheSameSave(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions don't stop this user from reading")
	}
	ws, root := newRefreshWorkspace(t, excluding("vendor", "legacy"), map[string]string{
		"legacy/Locked.php": "<?php\nclass LockedLegacy {}\n",
	})
	locked := filepath.Join(root, "legacy/Locked.php")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	writeFiles(t, root, map[string]string{config.FileName: `{"excludePaths": ["vendor"], "phpVersion": "7.4"}`})

	out, err := ws.Refresh(Refresh{Paths: []string{ws.ConfigPath()}, Config: func() (*config.Config, error) {
		return config.LoadFromFile(ws.ConfigPath())
	}})
	if err == nil || !strings.Contains(err.Error(), "index "+locked) {
		t.Errorf("the file that can't be read is reported: %v", err)
	}
	if configErr := (*ConfigError)(nil); errors.As(err, &configErr) {
		t.Errorf("a file the rescope can't read is no config error: %v", err)
	}
	if !out.ProfileChanged || ws.Profile().PHPVersion != "7.4" {
		t.Errorf("the PHP version saved with the new excludePaths applies: %+v", ws.Profile())
	}
}
