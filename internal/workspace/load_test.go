package workspace

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type cancelAfter struct {
	context.Context
	checks int
}

func (c *cancelAfter) Err() error {
	if c.checks == 0 {
		return context.Canceled
	}
	c.checks--
	return nil
}

func newLoadWorkspace(t *testing.T, framework string, files map[string]string) (*Bootstrapped, string) {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, files)
	return New(Options{RootPath: root, Framework: framework, Config: config.DefaultConfig(), Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}}), root
}

func TestLoadStopsAtEveryStepOnceTheContextIsCancelled(t *testing.T) {
	for checks := range 5 {
		t.Run(strconv.Itoa(checks), func(t *testing.T) {
			ws, _ := newLoadWorkspace(t, "none", map[string]string{"app/Order.php": "<?php\nclass Order {}\n"})
			if err := ws.Load(&cancelAfter{Context: context.Background(), checks: checks}); !errors.Is(err, context.Canceled) {
				t.Errorf("a cancelled load returns the context's error, got %v", err)
			}
			if ws.Index.Ready() {
				t.Error("a cancelled load leaves the index not ready")
			}
		})
	}
}

func TestLoadReportsARoutesDirectoryItCannotLookAt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links needs privileges on Windows")
	}
	ws, root := newLoadWorkspace(t, "laravel", nil)
	routes := filepath.Join(root, "routes")
	if err := os.Symlink(routes, routes); err != nil {
		t.Fatal(err)
	}
	if err := ws.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "routes") {
		t.Errorf("a routes directory that can't be looked at fails the load, got %v", err)
	}
	if ws.Index.Ready() {
		t.Error("a failed load leaves the index not ready")
	}
}

func TestVendorStatePathsAreWhatComposerWritesOnInstall(t *testing.T) {
	ws, root := newLoadWorkspace(t, "none", nil)
	want := []string{filepath.Join(root, "composer.lock"), filepath.Join(root, "vendor", "composer", "installed.json")}
	if got := ws.VendorStatePaths(); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestARefreshThatCannotLoadTheConfigurationSaysSo(t *testing.T) {
	ws, root := newRefreshWorkspace(t, nil, map[string]string{"composer.json": `{"require": {"php": "^8.2"}}`})
	broken := errors.New("unexpected end of JSON input")
	_, err := ws.Refresh(Refresh{
		Paths:  []string{filepath.Join(root, "composer.json")},
		Config: func() (*config.Config, error) { return nil, broken },
	})
	var configErr *ConfigError
	if !errors.As(err, &configErr) || !errors.Is(err, broken) {
		t.Fatalf("the configuration's error comes back as a ConfigError, got %v", err)
	}
	if want := "load the configuration to resolve the PHP version: unexpected end of JSON input"; err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}
