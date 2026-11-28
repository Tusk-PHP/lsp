package mcpserver

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func writeProject(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		later := time.Now().Add(time.Second)
		if err := os.Chtimes(path, later, later); err != nil {
			t.Fatal(err)
		}
	}
}

func connectedService(t *testing.T, files map[string]string) (*mcp.ClientSession, string) {
	t.Helper()
	root := t.TempDir()
	writeProject(t, root, files)
	svc, err := New(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ct, st := mcp.NewInMemoryTransports()
	serverSession, err := svc.Server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Wait() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, root
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (map[string]any, error) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		t.Fatalf("%s returned an error result: %+v", name, res.Content)
	}
	return structuredMap(t, res.StructuredContent), nil
}

func foundFQNs(t *testing.T, session *mcp.ClientSession, query string) []string {
	t.Helper()
	out, err := callTool(t, session, "php_find_symbol", map[string]any{"query": query})
	if err != nil {
		t.Fatal(err)
	}
	var fqns []string
	results, _ := out["results"].([]any)
	for _, r := range results {
		fqns = append(fqns, r.(map[string]any)["fqn"].(string))
	}
	return fqns
}

const plainProjectConfig = `{"framework": "none", "databaseEnabled": false}`

func TestToolCallsSeeProjectFilesChangedOnDisk(t *testing.T) {
	session, root := connectedService(t, map[string]string{
		".tusk-php.json": plainProjectConfig,
		"composer.json":  `{"require": {"php": "^8.2"}}`,
		"app/Old.php":    "<?php\nnamespace App;\nclass Old {}\n",
		"app/Edit.php":   "<?php\nnamespace App;\nclass Edit {}\n",
	})
	if got := foundFQNs(t, session, `App\`); !slices.Contains(got, `App\Old`) || !slices.Contains(got, `App\Edit`) {
		t.Fatalf("the project's classes are indexed: %v", got)
	}

	if err := os.Remove(filepath.Join(root, "app/Old.php")); err != nil {
		t.Fatal(err)
	}
	writeProject(t, root, map[string]string{
		"app/Added.php": "<?php\nnamespace App;\nclass Added {}\n",
		"app/Edit.php":  "<?php\nnamespace App;\nclass Edited {}\n",
	})
	got := foundFQNs(t, session, `App\`)
	want := []string{`App\Added`, `App\Edited`}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("the next tool call answers from the files on disk: got %v, want %v", got, want)
	}
}

func TestToolCallsFollowThePHPVersionInComposerJSON(t *testing.T) {
	session, root := connectedService(t, map[string]string{
		".tusk-php.json": plainProjectConfig,
		"composer.json":  `{"require": {"php": "^8.2"}}`,
		"check.php":      "<?php\nfunction check(string $s): bool { return str_contains($s, 'x'); }\n",
	})
	summary, err := callTool(t, session, "php_project_summary", nil)
	if err != nil || summary["phpVersion"] != "8.2" {
		t.Fatalf("the project starts on PHP 8.2: %v %v", summary, err)
	}

	writeProject(t, root, map[string]string{"composer.json": `{"require": {"php": "^7.4"}}`})
	summary, err = callTool(t, session, "php_project_summary", nil)
	if err != nil || summary["phpVersion"] != "7.4" || summary["phpVersionSource"] != "composer" {
		t.Fatalf("the summary reports composer.json's new PHP version: %v %v", summary, err)
	}
	if got := diagnosticCodes(t, session, "check.php"); !slices.Contains(got, "builtin-unavailable") {
		t.Errorf("str_contains is reported as unavailable on PHP 7.4: %v", got)
	}
}

func TestToolCallsSeeComposerInstallsAndRemovals(t *testing.T) {
	installed := func(name, ns string) string {
		return `{"packages": [{"name": "` + name + `", "install-path": "../` + name + `", "autoload": {"psr-4": {"` + ns + `\\": "src/"}}}]}`
	}
	session, root := connectedService(t, map[string]string{
		".tusk-php.json":                  plainProjectConfig,
		"composer.json":                   `{"require": {"php": "^8.2"}}`,
		"vendor/composer/installed.json":  installed("acme/old", "Old"),
		"vendor/acme/old/src/Retired.php": "<?php\nnamespace Old;\nclass Retired {}\n",
	})
	if got := foundFQNs(t, session, "Retired"); !slices.Contains(got, `Old\Retired`) {
		t.Fatalf("the installed package is indexed: %v", got)
	}

	if err := os.RemoveAll(filepath.Join(root, "vendor/acme/old")); err != nil {
		t.Fatal(err)
	}
	writeProject(t, root, map[string]string{
		"vendor/composer/installed.json": installed("acme/new", "Fresh"),
		"vendor/acme/new/src/Gadget.php": "<?php\nnamespace Fresh;\nclass Gadget {}\n",
	})
	if got := foundFQNs(t, session, "Gadget"); !slices.Contains(got, `Fresh\Gadget`) {
		t.Errorf("a package Composer installed is found: %v", got)
	}
	if got := foundFQNs(t, session, "Retired"); len(got) != 0 {
		t.Errorf("a package Composer removed is gone: %v", got)
	}
}

func diagnosticCodes(t *testing.T, session *mcp.ClientSession, path string) []string {
	t.Helper()
	out, err := callTool(t, session, "php_diagnostics", map[string]any{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, f := range out["files"].([]any) {
		for _, d := range f.(map[string]any)["diagnostics"].([]any) {
			codes = append(codes, d.(map[string]any)["code"].(string))
		}
	}
	return codes
}

func TestToolCallsFollowRuleSeveritiesChangedOnDisk(t *testing.T) {
	session, root := connectedService(t, map[string]string{
		".tusk-php.json": plainProjectConfig,
		"composer.json":  `{"require": {"php": "^8.2"}}`,
		"app/make.php":   "<?php\nnamespace App;\nfunction make() { return new Missing(); }\n",
	})
	if got := diagnosticCodes(t, session, "app/make.php"); !slices.Contains(got, "unknown-class") {
		t.Fatalf("the unknown class is reported with the startup configuration: %v", got)
	}

	writeProject(t, root, map[string]string{".tusk-php.json": `{"framework": "none", "databaseEnabled": false, "diagnosticRules": {"unknown-class": "off"}}`})
	if got := diagnosticCodes(t, session, "app/make.php"); slices.Contains(got, "unknown-class") {
		t.Errorf("a rule turned off in the config file on disk is no longer reported: %v", got)
	}
}

func TestToolCallsFailOnABrokenConfigFile(t *testing.T) {
	session, root := connectedService(t, map[string]string{
		".tusk-php.json": plainProjectConfig,
		"composer.json":  `{"name": "app/app"}`,
	})
	writeProject(t, root, map[string]string{".tusk-php.json": `{"phpVersion": `})
	if _, err := callTool(t, session, "php_project_summary", nil); err == nil {
		t.Error("a configuration that can't be read fails the call instead of being ignored")
	}
}

func TestTheConfigFilesPHPVersionApplies(t *testing.T) {
	session, _ := connectedService(t, map[string]string{
		".tusk-php.json": `{"framework": "none", "databaseEnabled": false, "phpVersion": "7.4"}`,
		"composer.json":  `{"name": "app/app"}`,
	})
	summary, err := callTool(t, session, "php_project_summary", nil)
	if err != nil || summary["phpVersion"] != "7.4" || summary["phpVersionSource"] != "config" {
		t.Errorf(".tusk-php.json's phpVersion applies when composer.json names none: %v %v", summary, err)
	}
}

func TestAnUnreadableFileDoesNotFailToolCalls(t *testing.T) {
	session, root := connectedService(t, map[string]string{
		".tusk-php.json": plainProjectConfig,
		"composer.json":  `{"require": {"php": "^8.2"}}`,
		"app/Order.php":  "<?php\nnamespace App;\nclass Order {}\n",
	})
	writeProject(t, root, map[string]string{"app/Locked.php": "<?php\nnamespace App;\nclass Locked {}\n"})
	locked := filepath.Join(root, "app/Locked.php")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o644) })
	for range 2 {
		if got := foundFQNs(t, session, `App\`); !slices.Contains(got, `App\Order`) {
			t.Errorf("tool calls keep answering while a file can't be read: %v", got)
		}
	}

	writeProject(t, root, map[string]string{"app/Added.php": "<?php\nnamespace App;\nclass Added {}\n"})
	if err := os.Chmod(locked, 0o644); err != nil {
		t.Fatal(err)
	}
	got := foundFQNs(t, session, `App\`)
	if !slices.Contains(got, `App\Added`) || !slices.Contains(got, `App\Locked`) {
		t.Errorf("other changes still apply, and the unreadable file is read once it can be: %v", got)
	}
}

type writerFunc func([]byte)

func (f writerFunc) Write(p []byte) (int, error) {
	f(p)
	return len(p), nil
}

func TestBuildRecordsTheFilesAsTheyWereBeforeIndexing(t *testing.T) {
	root := t.TempDir()
	writeProject(t, root, map[string]string{"app/Order.php": "<?php\nnamespace App;\nclass Order {}\n"})
	edited := false
	logger := log.New(writerFunc(func(p []byte) {
		if !edited && strings.HasPrefix(string(p), "Indexed ") && strings.Contains(string(p), "PHP files") {
			edited = true
			writeProject(t, root, map[string]string{"app/Order.php": "<?php\nnamespace App;\nclass RenamedDuringIndexing {}\n"})
		}
	}), "", 0)
	cfg := config.DefaultConfig()
	cfg.Framework = "none"
	ws, files, err := buildWorkspace(context.Background(), workspace.Options{RootPath: root, Framework: "none", Config: cfg, Logger: logger, BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	if err != nil {
		t.Fatal(err)
	}
	if !edited {
		t.Fatal("the file is edited after indexing read it")
	}
	if changed := changedFiles(files, scanDisk(ws)); !slices.Contains(changed, filepath.Join(root, "app/Order.php")) {
		t.Errorf("a file written while the index was built counts as changed on the next call: %v", changed)
	}
}

func TestReindexUsesTheConfigFileAsItIsNow(t *testing.T) {
	session, root := connectedService(t, map[string]string{
		".tusk-php.json":      plainProjectConfig,
		"composer.json":       `{"require": {"php": "^8.2"}}`,
		"legacy/OldOrder.php": "<?php\nnamespace Legacy;\nclass OldOrder {}\n",
	})
	if got := foundFQNs(t, session, "OldOrder"); !slices.Contains(got, `Legacy\OldOrder`) {
		t.Fatalf("the legacy class starts indexed: %v", got)
	}
	writeProject(t, root, map[string]string{".tusk-php.json": `{"framework": "none", "databaseEnabled": false, "excludePaths": ["vendor", "legacy"]}`})
	if _, err := callTool(t, session, reindexTool, nil); err != nil {
		t.Fatal(err)
	}
	if got := foundFQNs(t, session, "OldOrder"); len(got) != 0 {
		t.Errorf("a rebuild indexes with the configuration it read: %v", got)
	}
}

func BenchmarkSyncWithDiskUnchanged(b *testing.B) {
	for _, files := range []int{1000, 10000} {
		b.Run(strconv.Itoa(files), func(b *testing.B) {
			root := b.TempDir()
			for i := range files {
				dir := filepath.Join(root, "app", "m"+strconv.Itoa(i/50))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					b.Fatal(err)
				}
				src := "<?php\nnamespace App;\nclass C" + strconv.Itoa(i) + " {}\n"
				if err := os.WriteFile(filepath.Join(dir, "C"+strconv.Itoa(i)+".php"), []byte(src), 0o644); err != nil {
					b.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte(plainProjectConfig), 0o644); err != nil {
				b.Fatal(err)
			}
			svc, err := New(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if err := svc.syncWithDisk(context.Background()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
