package phpref

import (
	"bufio"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type PHP struct {
	Command []string
	Version token.Version
}

func Versions(t testing.TB) []PHP {
	t.Helper()
	commands := os.Getenv("TUSK_PHP_TOKENIZERS")
	if commands == "" {
		commands = "php"
	}
	var refs []PHP
	for _, command := range strings.Split(commands, ";") {
		argv := strings.Fields(command)
		out, err := exec.Command(argv[0], append(argv[1:], "-r", `if (!function_exists("token_get_all")) exit(1); echo PHP_MAJOR_VERSION, ".", PHP_MINOR_VERSION;`)...).Output()
		if err != nil {
			t.Logf("skipping %s: no PHP with the tokenizer extension: %v", command, err)
			continue
		}
		version, err := token.ParseVersion(string(out))
		if err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if version.String() != string(out) {
			t.Logf("skipping %s: PHP %s is outside the lexer's %s to %s", command, out, token.PHP74, token.Latest)
			continue
		}
		refs = append(refs, PHP{argv, version})
	}
	if len(refs) == 0 {
		skipOutsideCI(t, "no PHP "+token.PHP74.String()+" to "+token.Latest.String()+" with the tokenizer extension (set TUSK_PHP_TOKENIZERS to commands separated by ';')")
	}
	return refs
}

func skipOutsideCI(t testing.TB, reason string) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Fatal(reason)
	}
	t.Skip(reason)
}

func Corpus(t testing.TB) []string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "testdata")
	var files []string
	for _, dir := range []string{"project", "laravel/vendor", "symfony/vendor"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".php") {
				files = append(files, path)
			}
			return nil
		})
	}
	if len(files) < 100 {
		skipOutsideCI(t, "the Laravel and Symfony fixtures' vendor trees aren't installed")
	}
	return files
}

func (php PHP) Run(t testing.TB, script string, sources map[string]string, args ...string) map[string]string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.php")
	full := "<?php\nregister_shutdown_function(function () use (&$path) { $e = error_get_last(); if ($e && $e['type'] === E_COMPILE_ERROR) { echo $path, \"\\tfatal: \", str_replace(\"\\n\", ' ', $e['message']), \"\\n\"; } });\nwhile (($path = fgets(STDIN)) !== false) {\n    $path = rtrim($path, \"\\n\");\n    $length = (int) fgets(STDIN);\n    $source = $length > 0 ? stream_get_contents(STDIN, $length) : '';\n" + script + "\n}\n"
	if err := os.WriteFile(path, []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}
	cmdArgs := append(append(append([]string{}, php.Command[1:]...), "-d", "short_open_tag=0", "-d", "memory_limit=-1", "-d", "display_errors=stderr", path), args...)
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	results := map[string]string{}
	for len(names) > 0 {
		cmd := exec.Command(php.Command[0], cmdArgs...)
		var input strings.Builder
		for _, name := range names {
			input.WriteString(name + "\n" + strconv.Itoa(len(sources[name])) + "\n" + sources[name])
		}
		cmd.Stdin = strings.NewReader(input.String())
		out, err := cmd.Output()
		fatal := ""
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		scanner.Buffer(nil, 1<<30)
		for scanner.Scan() {
			name, result, _ := strings.Cut(scanner.Text(), "\t")
			results[name] = result
			if strings.HasPrefix(result, "fatal: ") {
				fatal = name
			}
		}
		if err == nil {
			break
		}
		if fatal == "" {
			t.Fatalf("running PHP %s: %v", php.Version, err)
		}
		remaining := names[:0]
		for _, name := range names {
			if _, done := results[name]; !done {
				remaining = append(remaining, name)
			}
		}
		names = remaining
	}
	return results
}

func ReadAll(t testing.TB, paths []string) map[string]string {
	t.Helper()
	sources := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources[path] = string(data)
	}
	return sources
}
