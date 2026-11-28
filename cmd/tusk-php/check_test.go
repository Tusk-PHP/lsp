//go:build !wasm

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func check(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code, err := runCheck(args, &out, &errOut)
	if err != nil {
		t.Fatalf("runCheck(%v): %v\n%s", args, err, errOut.String())
	}
	return code, out.String(), errOut.String()
}

// check reports what the server would publish, with paths relative to the
// root and 1-based positions, and fails on warnings; a baseline lets through
// what it lists, matched without positions.
func TestCheckCommand(t *testing.T) {
	root := writeProject(t, map[string]string{
		"composer.json":           `{"autoload": {"psr-4": {"App\\": "src/"}}}`,
		"src/Clean.php":           "<?php\nnamespace App;\nclass Clean { public function run(): int { return 1; } }\n",
		"src/Bad.php":             "<?php\nnamespace App;\nclass Bad {\n    public function run(): int\n    {\n        return $missing;\n    }\n}\n",
		"vendor/acme/lib/Lib.php": "<?php\nnamespace Acme;\nclass Lib { public function run() { return $nope; } }\n",
	})

	code, out, _ := check(t, "--root", root)
	want := "src/Bad.php:6:16: warning undefined-variable: Undefined variable $missing\n"
	if code != 1 || out != want {
		t.Fatalf("whole project: exit %d, output\n%s\nwant exit 1 and\n%s", code, out, want)
	}

	if code, out, _ := check(t, "--root", root, "src/Clean.php"); code != 0 || out != "" {
		t.Errorf("one clean file: exit %d, output %q", code, out)
	}
	if _, out, _ := check(t, "--root", root, "src"); out != want {
		t.Errorf("a directory: %q", out)
	}
	if _, out, _ := check(t, "--root", root, "src/B*.php"); out != want {
		t.Errorf("a glob: %q", out)
	}

	_, out, _ = check(t, "--root", root, "--format", "json")
	var findings []finding
	if err := json.Unmarshal([]byte(out), &findings); err != nil || len(findings) != 1 || findings[0].Code != "undefined-variable" || findings[0].Line != 6 {
		t.Errorf("json: %v %+v", err, findings)
	}

	baseline := filepath.Join(t.TempDir(), "baseline.txt")
	check(t, "--root", root, "--write-baseline", baseline)
	if code, out, _ := check(t, "--root", root, "--baseline", baseline); code != 0 || out != "" {
		t.Errorf("against its own baseline: exit %d, output %q", code, out)
	}

	// The known finding moves down a line and a new one appears: only the
	// new one is reported.
	moved := "<?php\nnamespace App;\n\nclass Bad {\n    public function run(): int\n    {\n        return $missing + $other;\n    }\n}\n"
	if err := os.WriteFile(filepath.Join(root, "src/Bad.php"), []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, summary := check(t, "--root", root, "--baseline", baseline)
	if code != 1 || !strings.Contains(out, "$other") || strings.Contains(out, "$missing") {
		t.Errorf("new finding: exit %d, output %q", code, out)
	}
	if !strings.Contains(summary, "1 not in the baseline, 0 in the baseline no longer found") {
		t.Errorf("summary: %q", summary)
	}
}

// --exclude leaves out files by a directory or file name anywhere in the path,
// or by a path from the root.
func TestCheckExclude(t *testing.T) {
	root := writeProject(t, map[string]string{
		"composer.json":              `{"autoload": {"psr-4": {"App\\": "src/"}}}`,
		"src/Bad.php":                "<?php\nfunction a() { return $one; }\n",
		"src/Tests/BadTest.php":      "<?php\nfunction b() { return $two; }\n",
		"src/Mail/Tests/Fixture.php": "<?php\nfunction c() { return $three; }\n",
		"src/Legacy/Old.php":         "<?php\nfunction d() { return $four; }\n",
	})
	files := func(args ...string) string {
		t.Helper()
		_, out, _ := check(t, append([]string{"--root", root}, args...)...)
		var names []string
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line != "" {
				names = append(names, line[:strings.Index(line, ":")])
			}
		}
		return strings.Join(names, " ")
	}
	if got := files("--exclude", "Tests"); got != "src/Bad.php src/Legacy/Old.php" {
		t.Errorf("by directory name: %q", got)
	}
	if got := files("--exclude", "*Test.php", "--exclude", "src/Legacy"); got != "src/Bad.php src/Mail/Tests/Fixture.php" {
		t.Errorf("by file name and path: %q", got)
	}
	if got := files("--exclude", "src/*/Tests"); got != "src/Bad.php src/Legacy/Old.php src/Tests/BadTest.php" {
		t.Errorf("by a glob path: %q", got)
	}
}

// check also runs the rules the editor runs on save, such as
// redundant-nullsafe.
func TestCheckRunsOnSaveRules(t *testing.T) {
	root := writeProject(t, map[string]string{
		"composer.json": `{"autoload": {"psr-4": {"App\\": "src/"}}}`,
		"src/Service.php": "<?php\nnamespace App;\nclass Logger { public function info(): void {} }\n" +
			"class Service {\n    public function run(Logger $logger): void\n    {\n        $logger?->info();\n    }\n}\n",
	})
	_, out, _ := check(t, "--root", root)
	want := "src/Service.php:7:16: info redundant-nullsafe: Redundant nullsafe operator: type 'Logger' is non-nullable\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestCheckReportsSkippedConfiguration(t *testing.T) {
	root := writeProject(t, map[string]string{
		".tusk-php.json": `{"diagnosticIgnore":[{"code":"unknown-members"}]}`,
		"src/Clean.php":  "<?php\nclass Clean {}\n",
	})
	_, _, errOut := check(t, "--root", root)
	if !strings.Contains(errOut, `skipped part of the configuration: diagnosticIgnore code: unknown diagnostic rule "unknown-members"`) {
		t.Errorf("stderr: %s", errOut)
	}
}

func TestProjectMetadataReportsSkippedConfiguration(t *testing.T) {
	root := writeProject(t, map[string]string{".tusk-php.json": `{"diagnosticIgnore":[{"path":"./"}]}`})
	var errOut bytes.Buffer
	if _, _, _, err := loadProjectMetadata(root, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "tusk-php: skipped part of the configuration: diagnosticIgnore path") {
		t.Errorf("stderr: %s", errOut.String())
	}
}

func TestBuildWorkspaceIndexesTheRoot(t *testing.T) {
	root := writeProject(t, map[string]string{"src/Shop.php": "<?php\nnamespace App;\nclass Shop {}\n"})
	ws, _, err := buildWorkspace(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if ws.Index.Lookup(`App\Shop`) == nil {
		t.Error("the root's classes are indexed")
	}
	broken := writeProject(t, map[string]string{".tusk-php.json": "{"})
	if _, _, err := buildWorkspace(broken, "test"); err == nil || !strings.Contains(err.Error(), "loading project metadata") {
		t.Errorf("a configuration that doesn't load stops the build: %v", err)
	}
}

func TestCheckUsesTheConfigFilesPHPVersion(t *testing.T) {
	root := writeProject(t, map[string]string{"composer.json": `{"name": "app/app"}`, ".tusk-php.json": `{"phpVersion": "7.4"}`})
	var errOut bytes.Buffer
	_, _, profile, err := loadProjectMetadata(root, &errOut)
	if err != nil || profile.PHPVersion != "7.4" {
		t.Errorf(".tusk-php.json's phpVersion applies when composer.json names none: %+v %v", profile, err)
	}
}

func symlinkedProject(t *testing.T) (root, link string) {
	t.Helper()
	root = writeProject(t, map[string]string{
		"composer.json":                  `{"autoload": {"psr-4": {"App\\": "src/"}}}`,
		"vendor/composer/installed.json": `{"packages": [{"name": "acme/lib", "install-path": "../acme/lib", "autoload": {"psr-4": {"Acme\\": "src/"}}}]}`,
		"vendor/acme/lib/src/Lib.php":    "<?php\nnamespace Acme;\nclass Lib { public function run() { return $nope; } }\n",
		"src/Uses.php":                   "<?php\nnamespace App;\nclass Uses {\n    public function run(): int\n    {\n        new \\Acme\\Lib();\n        new \\Acme\\Missing();\n        return $missing;\n    }\n}\n",
		"src/Clean.php":                  "<?php\nnamespace App;\nclass Clean { public function run(): int { return 1; } }\n",
	})
	link = filepath.Join(t.TempDir(), "app")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return root, link
}

func TestCheckThroughASymlinkedRoot(t *testing.T) {
	root, link := symlinkedProject(t)
	want := "src/Uses.php:7:13: warning unknown-class: Unknown class 'Acme\\Missing'\n" +
		"src/Uses.php:8:16: warning undefined-variable: Undefined variable $missing\n"
	for _, args := range [][]string{
		{"--root", root},
		{"--root", link},
		{"--root", link, "src"},
		{"--root", link, filepath.Join(link, "src")},
		{"--root", root, filepath.Join(link, "src", "Uses.php")},
		{"--root", link, "src/U*.php"},
	} {
		_, out, summary := check(t, args...)
		if out != want {
			t.Errorf("check %v:\n%s\nwant\n%s", args, out, want)
		}
		if strings.HasPrefix(summary, "0 files") {
			t.Errorf("check %v checked no files: %s", args, summary)
		}
	}
}
