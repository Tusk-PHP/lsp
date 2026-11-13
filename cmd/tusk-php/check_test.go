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
