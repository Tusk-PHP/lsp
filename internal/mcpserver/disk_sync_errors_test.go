package mcpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestNewStopsWhenTheContextIsCancelled(t *testing.T) {
	root := t.TempDir()
	writeProject(t, root, map[string]string{".tusk-php.json": plainProjectConfig, "app/Order.php": "<?php\nclass Order {}\n"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(ctx, root, slog.New(slog.NewTextHandler(io.Discard, nil))); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled start returns the context's error, got %v", err)
	}
}

func TestAComposerChangeWithABrokenConfigFileFailsTheCall(t *testing.T) {
	session, root := connectedService(t, map[string]string{
		".tusk-php.json": plainProjectConfig,
		"composer.json":  `{"require": {"php": "^8.2"}}`,
		"composer.lock":  `{"packages": []}`,
	})
	writeProject(t, root, map[string]string{
		"composer.lock":  `{"packages": [], "content-hash": "changed"}`,
		".tusk-php.json": `{"framework": `,
	})
	_, err := callTool(t, session, "php_project_summary", nil)
	if err == nil || !strings.Contains(err.Error(), "rebuild the index after a Composer change") {
		t.Errorf("a rebuild that can't read the configuration fails the call and says why, got %v", err)
	}
}

func TestAnIndexedFileThatTurnsUnreadableIsReadOnceItCanBe(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions don't stop this user from reading")
	}
	session, root := connectedService(t, map[string]string{
		".tusk-php.json": plainProjectConfig,
		"composer.json":  `{"require": {"php": "^8.2"}}`,
		"app/Order.php":  "<?php\nnamespace App;\nclass Order {}\n",
	})
	if got := foundFQNs(t, session, `App\`); !slices.Contains(got, `App\Order`) {
		t.Fatalf("the class starts indexed: %v", got)
	}
	writeProject(t, root, map[string]string{"app/Order.php": "<?php\nnamespace App;\nclass Invoice {}\n"})
	order := filepath.Join(root, "app/Order.php")
	if err := os.Chmod(order, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(order, 0o644) })
	foundFQNs(t, session, `App\`)

	if err := os.Chmod(order, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := foundFQNs(t, session, `App\`); !slices.Contains(got, `App\Invoice`) {
		t.Errorf("a file that failed to re-index is read on a later call even though it was not written again: %v", got)
	}
}
