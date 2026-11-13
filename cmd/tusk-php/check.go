//go:build !wasm

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tusk-PHP/lsp/internal/mcpserver"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// finding is one diagnostic in a checked file, with a path relative to the
// project root and a 1-based line and column, as linters print them.
type finding struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// baselineKey leaves the position out, so a baseline survives edits that
// move code around.
func (f finding) baselineKey() string {
	return f.Path + "\t" + f.Code + "\t" + f.Message
}

// runCheck implements the `check` subcommand: it reports the diagnostics the
// language server would publish for a project's files, with the providers
// wired the same way (the project's PHP version and extensions, framework and
// .tusk-php.json), and exits non-zero when there are errors or warnings, or,
// with --baseline, findings the baseline doesn't list.
func runCheck(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rootFlag := fs.String("root", "", "Project root (default: current directory)")
	format := fs.String("format", "text", "Output format: text|json")
	baseline := fs.String("baseline", "", "Report only findings this baseline file doesn't list")
	writeBaseline := fs.String("write-baseline", "", "Write all findings to this baseline file")
	var excludes []string
	fs.Func("exclude", "Leave out files under a directory or with a name matching this glob (repeatable)", func(v string) error {
		if _, err := path.Match(v, ""); err != nil {
			return fmt.Errorf("bad pattern %q: %w", v, err)
		}
		excludes = append(excludes, strings.Trim(filepath.ToSlash(v), "/"))
		return nil
	})
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tusk-php check [--root=.] [--format=text|json] [--exclude=glob ...] [--baseline=file] [--write-baseline=file] [path|dir|glob ...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if *format != "text" && *format != "json" {
		return 2, fmt.Errorf("invalid --format %q: must be text or json", *format)
	}
	root := *rootFlag
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return 2, err
	}

	start := time.Now()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := mcpserver.New(context.Background(), root, logger)
	if err != nil {
		return 2, fmt.Errorf("building workspace: %w", err)
	}
	files, err := checkTargets(root, fs.Args(), svc.Workspace.Index)
	if err != nil {
		return 2, err
	}
	files = excludeFiles(root, files, excludes)
	indexed := time.Since(start)

	var all []finding
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel := relativePath(root, path)
		for _, d := range svc.Diagnostics.Analyze("file://"+path, string(data)) {
			all = append(all, finding{
				Path:     rel,
				Line:     d.Range.Start.Line + 1,
				Column:   d.Range.Start.Character + 1,
				Severity: severityName(d.Severity),
				Code:     d.Code,
				Message:  d.Message,
			})
		}
	}
	sortFindings(all)

	if *writeBaseline != "" {
		if err := writeBaselineFile(*writeBaseline, all); err != nil {
			return 2, err
		}
	}
	report := all
	fixed := 0
	if *baseline != "" {
		known, err := readBaselineFile(*baseline)
		if err != nil {
			return 2, err
		}
		report, fixed = newFindings(all, known)
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if report == nil {
			report = []finding{}
		}
		if err := enc.Encode(report); err != nil {
			return 2, err
		}
	default:
		for _, f := range report {
			fmt.Fprintf(stdout, "%s:%d:%d: %s %s: %s\n", f.Path, f.Line, f.Column, f.Severity, f.Code, f.Message)
		}
	}

	summary := fmt.Sprintf("%d files, %d findings", len(files), len(all))
	if *baseline != "" {
		summary += fmt.Sprintf(", %d not in the baseline, %d in the baseline no longer found", len(report), fixed)
	}
	fmt.Fprintf(stderr, "%s (indexed in %s, checked in %s)\n", summary, indexed.Round(time.Millisecond), (time.Since(start) - indexed).Round(time.Millisecond))

	if *baseline != "" {
		if len(report) > 0 {
			return 1, nil
		}
		return 0, nil
	}
	for _, f := range report {
		if f.Severity == "error" || f.Severity == "warning" {
			return 1, nil
		}
	}
	return 0, nil
}

// checkTargets lists the files to check: the project's own indexed files
// (not vendor/) when no paths are given; otherwise each file, each directory's
// indexed files, and each glob's matches, relative to root.
func checkTargets(root string, args []string, index *symbols.Index) ([]string, error) {
	var indexed []string
	for _, uri := range index.GetAllFileURIs() {
		if path := symbols.URIToPath(uri); path != "" && workspace.IsPHPSource(path) {
			indexed = append(indexed, path)
		}
	}
	under := func(dir string) []string {
		var out []string
		prefix := strings.TrimSuffix(dir, string(filepath.Separator)) + string(filepath.Separator)
		for _, path := range indexed {
			if strings.HasPrefix(path, prefix) {
				out = append(out, path)
			}
		}
		return out
	}
	seen := map[string]bool{}
	var files []string
	add := func(paths ...string) {
		for _, p := range paths {
			if !seen[p] {
				seen[p] = true
				files = append(files, p)
			}
		}
	}
	if len(args) == 0 {
		vendor := filepath.Join(root, "vendor") + string(filepath.Separator)
		for _, path := range under(root) {
			if !strings.HasPrefix(path, vendor) {
				add(path)
			}
		}
	}
	for _, arg := range args {
		pattern := arg
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(root, pattern)
		}
		matches := []string{pattern}
		if strings.ContainsAny(arg, "*?[") {
			var err error
			if matches, err = filepath.Glob(pattern); err != nil {
				return nil, fmt.Errorf("bad pattern %q: %w", arg, err)
			}
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			switch {
			case err != nil:
				return nil, fmt.Errorf("%s: %w", arg, err)
			case info.IsDir():
				add(under(m)...)
			case workspace.IsPHPSource(m):
				add(m)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

// excludeFiles drops the files an --exclude pattern matches. A pattern
// without a slash matches any one directory or file name in the path, as in
// "Tests" or "*Test.php"; a pattern with one matches the path relative to root
// or a directory leading to it, as in "src/wp-content" or "app/code/*/Test".
func excludeFiles(root string, files, patterns []string) []string {
	if len(patterns) == 0 {
		return files
	}
	kept := files[:0:0]
	for _, file := range files {
		if !excluded(relativePath(root, file), patterns) {
			kept = append(kept, file)
		}
	}
	return kept
}

func excluded(rel string, patterns []string) bool {
	segments := strings.Split(rel, "/")
	for _, pattern := range patterns {
		if !strings.Contains(pattern, "/") {
			for _, segment := range segments {
				if ok, _ := path.Match(pattern, segment); ok {
					return true
				}
			}
			continue
		}
		depth := strings.Count(pattern, "/") + 1
		if depth <= len(segments) {
			if ok, _ := path.Match(pattern, strings.Join(segments[:depth], "/")); ok {
				return true
			}
		}
	}
	return false
}

func relativePath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func severityName(s protocol.DiagnosticSeverity) string {
	switch s {
	case protocol.DiagnosticSeverityError:
		return "error"
	case protocol.DiagnosticSeverityWarning:
		return "warning"
	case protocol.DiagnosticSeverityInformation:
		return "info"
	default:
		return "hint"
	}
}

func sortFindings(fs []finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		return a.Code < b.Code
	})
}

// A baseline file lists one finding per line as path, code and message
// separated by tabs, without positions. The same line twice means the finding
// is there twice.
func writeBaselineFile(path string, fs []finding) error {
	lines := make([]string, len(fs))
	for i, f := range fs {
		lines[i] = f.baselineKey()
	}
	sort.Strings(lines)
	body := strings.Join(lines, "\n")
	if body != "" {
		body += "\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func readBaselineFile(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	known := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			known[line]++
		}
	}
	return known, sc.Err()
}

// newFindings returns the findings the baseline doesn't account for, and how
// many it lists that weren't found.
func newFindings(all []finding, known map[string]int) ([]finding, int) {
	left := make(map[string]int, len(known))
	for k, n := range known {
		left[k] = n
	}
	var fresh []finding
	for _, f := range all {
		if left[f.baselineKey()] > 0 {
			left[f.baselineKey()]--
			continue
		}
		fresh = append(fresh, f)
	}
	fixed := 0
	for _, n := range left {
		fixed += n
	}
	return fresh, fixed
}
