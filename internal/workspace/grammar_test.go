package workspace

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const php74Declarations = `<?php
#[ a comment before PHP 8
function match($value) {
    return $value;
}
#[Marker]
class After {}
`

func TestTheIndexReadsCodeWithTheProjectsPHPVersion(t *testing.T) {
	cases := []struct {
		name     string
		composer string
		php      string
		source   string
		grammars string
		file     token.Version
	}{
		{name: "composer", composer: `{"require":{"php":"^7.4"}}`, source: "composer", grammars: "7.4", file: token.PHP74},
		{name: "the PHP on PATH", php: "echo 7.4", source: "local", grammars: "7.4", file: token.PHP74},
		{name: "composer on PHP 8.1 only", composer: `{"require":{"php":"~8.1.0"}}`, source: "composer", grammars: "8.1", file: token.PHP81},
		{name: "composer on PHP 8", composer: `{"require":{"php":"^8.0"}}`, source: "composer", grammars: "8.0-8.6", file: token.Latest},
		{name: "a range down to PHP 7.4 reads the file as the PHP it parses on", composer: `{"require":{"php":">=7.4"}}`, source: "composer", grammars: "7.4-8.6", file: token.PHP74},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.composer != "" {
				writeTestFile(t, filepath.Join(dir, "composer.json"), tc.composer)
			}
			binary := filepath.Join(dir, "missing-php")
			if tc.php != "" {
				binary = filepath.Join(dir, "php")
				writeExecutable(t, binary, "#!/bin/sh\n"+tc.php+"\n")
			}
			profile, source := ResolveBuiltinProfileWithConfig(dir, binary, 5*time.Second, nil, nil)
			if source != tc.source {
				t.Fatalf("source = %q, want %q", source, tc.source)
			}
			ws := New(Options{RootPath: dir, Framework: "none", BuiltinProfile: profile})
			if got := ws.Index.Grammars().String(); got != tc.grammars {
				t.Fatalf("grammars = %s, want %s", got, tc.grammars)
			}
			uri := "file://" + filepath.Join(dir, "a.php")
			ws.Index.IndexFile(uri, php74Declarations)
			if got := chosenGrammar(ws, uri); got != tc.file {
				t.Errorf("a.php is read as PHP %s, want %s", got, tc.file)
			}
			match, after := ws.Index.Lookup("match"), ws.Index.Lookup("After")
			if tc.file == token.PHP74 {
				if match == nil || match.Kind != symbols.KindFunction {
					t.Errorf("function match() = %+v, want it indexed", match)
				}
				if after == nil || after.Attribute("Marker") != nil {
					t.Errorf("class After = %+v, want it indexed without attributes", after)
				}
				return
			}
			if match != nil {
				t.Errorf("function match() indexed on PHP %s, where match is reserved", tc.file)
			}
		})
	}
}

func TestAProjectOfUnknownVersionReadsEachFileAsThePHPItParsesOn(t *testing.T) {
	dir := t.TempDir()
	profile, source := ResolveBuiltinProfileWithConfig(dir, filepath.Join(dir, "missing-php"), time.Second, nil, nil)
	if source != "fallback" {
		t.Fatalf("source = %q, want the fallback", source)
	}
	ws := New(Options{RootPath: dir, Framework: "none", BuiltinProfile: profile})
	files := []struct {
		name, source string
		grammar      token.Version
	}{
		{"php74.php", "<?php\nfunction match($value) { return $value; }\n", token.PHP74},
		{"php8.php", "<?php\nenum Suit { case Hearts; }\n#[Marker]\nclass Card { public function __construct(public readonly Suit $suit) {} }\n", token.Latest},
	}
	for _, f := range files {
		uri := "file://" + filepath.Join(dir, f.name)
		ws.Index.IndexFile(uri, f.source)
		got := chosenGrammar(ws, uri)
		if got != f.grammar {
			t.Errorf("%s is read as PHP %s, want %s", f.name, got, f.grammar)
		}
		if tree := phpparser.Parse(f.source, got); len(tree.Errors) > 0 {
			t.Errorf("%s has syntax errors on PHP %s: %v", f.name, f.grammar, tree.Errors)
		}
	}
	if ws.Index.Lookup("match") == nil {
		t.Error("function match() of php74.php is not indexed")
	}
	if card := ws.Index.Lookup("Card"); card == nil || card.Attribute("Marker") == nil {
		t.Errorf("class Card = %+v, want its #[Marker]", card)
	}
}

func chosenGrammar(ws *Bootstrapped, uri string) token.Version {
	return ws.Index.GrammarOf(uri).Parse(ws.Index.GetFileSource(uri)).Version
}

func TestAnEditedFileKeepsItsGrammarUntilTheNewestReadsItCleanly(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"php":">=7.4"}}`)
	profile, _ := ResolveBuiltinProfileWithConfig(dir, "", time.Second, nil, nil)
	ws := New(Options{RootPath: dir, Framework: "none", BuiltinProfile: profile})
	uri := "file://" + filepath.Join(dir, "a.php")
	edits := []struct {
		name, source string
		want         token.Version
	}{
		{"a PHP 7.4-only file", "<?php\nfunction match($value) { return $value; }\n", token.PHP74},
		{"typing in it", "<?php\nfunction match($value) { return $value\n", token.PHP74},
		{"PHP 8 code that the newest grammar reads cleanly", "<?php\n#[Marker]\nclass Card { public function f(int|string $a) {} }\n", token.Latest},
		{"typing in that", "<?php\n#[Marker]\nclass Card { public function f(int|string $a) { $this-> } }\n", token.Latest},
		{"PHP 7.4-only code again", "<?php\nfunction match($value) { return $value; }\n", token.Latest},
	}
	for _, edit := range edits {
		ws.Index.IndexFile(uri, edit.source)
		if got := chosenGrammar(ws, uri); got != edit.want {
			t.Errorf("%s: read as PHP %s, want %s", edit.name, got, edit.want)
		}
	}
}

func TestAFileBeingTypedKeepsTheNewestGrammarsTree(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"php":">=7.4"}}`)
	profile, _ := ResolveBuiltinProfileWithConfig(dir, "", time.Second, nil, nil)
	ws := New(Options{RootPath: dir, Framework: "none", BuiltinProfile: profile})
	uri := "file://" + filepath.Join(dir, "a.php")
	ws.Index.IndexFile(uri, "<?php\n#[Marker]\nclass Typing {\n    public function run(): void {\n        $this->\n    }\n}\n")
	if got := chosenGrammar(ws, uri); got != token.Latest {
		t.Errorf("a.php is read as PHP %s while typing, want %s", got, token.Latest)
	}
	if typing := ws.Index.Lookup("Typing"); typing == nil || typing.Attribute("Marker") == nil {
		t.Errorf("class Typing = %+v while typing, want its #[Marker]", typing)
	}
}

func TestTheIndexReadsAttributesOfAProjectThatAlsoRunsOnPHP8(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"php":">=7.4"}}`)
	profile, _ := ResolveBuiltinProfileWithConfig(dir, "", time.Second, nil, nil)
	ws := New(Options{RootPath: dir, Framework: "none", BuiltinProfile: profile})
	ws.Index.IndexFile("file://"+filepath.Join(dir, "a.php"), "<?php\n#[Marker] class After {}\n")
	if after := ws.Index.Lookup("After"); after == nil || after.Attribute("Marker") == nil {
		t.Errorf("class After = %+v, want its #[Marker]", after)
	}
}

func TestAConfiguredPHPVersionThatIsNotMajorMinorIsReportedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	cfg := configWithExplicitPHP(t, "eight")
	var logged []string
	profile, source := ResolveBuiltinProfileWithConfig(dir, filepath.Join(dir, "missing-php"), time.Second, cfg, func(msg string) { logged = append(logged, msg) })
	if source != "fallback" || profile.PHPVersion == "eight" {
		t.Errorf("profile %+v from %s, want the fallback", profile, source)
	}
	if len(logged) != 2 {
		t.Errorf("logged %q, want the skipped phpVersion and the provenance", logged)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writeExecutable(%q): %v", path, err)
	}
}

func TestBuiltinsKeepTheOldestPHPComposerAdmits(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"php":">=7.4"}}`)
	profile, _ := ResolveBuiltinProfileWithConfig(dir, "", time.Second, nil, nil)
	if profile.PHPVersion != "7.4" || profile.Grammars != phpparser.GrammarsBetween(token.PHP74, token.Latest) {
		t.Errorf("profile = %+v, want builtins for 7.4 and the latest grammar", profile)
	}
	ws := New(Options{RootPath: dir, Framework: "none", BuiltinProfile: profile})
	if ws.Index.Lookup("str_contains") != nil {
		t.Error("str_contains (PHP 8.0) is a builtin of a project that still runs on 7.4")
	}
}

func TestARefreshedProfileBringsItsGrammars(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"php":"^7.4"}}`)
	profile, _ := ResolveBuiltinProfileWithConfig(dir, "", time.Second, nil, nil)
	ws := New(Options{RootPath: dir, Framework: "none", BuiltinProfile: profile})

	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"php":"^7.4 || ^8.0"}}`)
	out := refreshPaths(t, ws, dir, "composer.json")
	if out.ProfileChange != GrammarsChanged || out.Profile.PHPVersion != "7.4" {
		t.Fatalf("a constraint that admits PHP 8 as well changes the profile: %+v", out)
	}
	if got := ws.Index.Grammars().String(); got != "7.4-8.6" {
		t.Fatalf("grammars = %s, want 7.4-8.6", got)
	}
	uri := "file://" + filepath.Join(dir, "b.php")
	ws.Index.IndexFile(uri, "<?php\n#[Marker]\nclass Card {}\n")
	if got := chosenGrammar(ws, uri); got != token.Latest {
		t.Errorf("b.php is read as PHP %s, want %s", got, token.Latest)
	}
}

func TestAVersionThePHPOnPathReportsButNoGrammarReadsIsIgnored(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "php")
	writeExecutable(t, binary, "#!/bin/sh\necho 999.1\n")
	var logs []string
	profile, source := ResolveBuiltinProfileWithConfig(dir, binary, 5*time.Second, nil, func(line string) { logs = append(logs, line) })
	if source != "fallback" || profile.Grammars != phpparser.AnyGrammar() {
		t.Errorf("an unreadable local version falls back to any grammar: %s %+v", source, profile)
	}
	if !slices.ContainsFunc(logs, func(line string) bool {
		return strings.HasPrefix(line, "PHP LSP: ignoring the version of the php on PATH") && strings.Contains(line, "999.1")
	}) {
		t.Errorf("the ignored version is logged: %q", logs)
	}
}

func TestFilesIndexedBeforeAPHPVersionChangeAreReadAgainUnderTheNewGrammars(t *testing.T) {
	const enumFile = "<?php\nenum Suit: string { case Hearts = 'h'; }\n"
	cases := []struct {
		name, from, to string
		files          map[string]string
		reparsed       int
		want           []string
	}{
		{
			name: "PHP 8.0 to 8.1 brings enums", from: "~8.0.0", to: "~8.1.0",
			files:    map[string]string{"app/Suit.php": enumFile, "_ide_helper.php": "<?php\nclass HelperCard { public function __construct(public readonly int $n) {} }\n"},
			reparsed: 2, want: []string{"Suit", "Suit::Hearts", "HelperCard"},
		},
		{
			name: "a range that already reads enums keeps the files", from: "^8.0", to: "^8.1",
			files:    map[string]string{"app/Suit.php": enumFile},
			reparsed: 0, want: []string{"Suit", "Suit::Hearts"},
		},
		{
			name: "PHP 8 to 7.4 frees the name match", from: "^8.0", to: "^7.4",
			files:    map[string]string{"app/match.php": "<?php\nfunction match($value) { return $value; }\n"},
			reparsed: 1, want: []string{"match"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"composer.json": `{"require":{"php":"` + tc.from + `"}}`}
			for name, source := range tc.files {
				files[name] = source
			}
			root := t.TempDir()
			writeFiles(t, root, files)
			profile, _ := ResolveBuiltinProfileWithConfig(root, "", time.Second, nil, nil)
			ws := New(Options{RootPath: root, Framework: "none", Logger: log.New(io.Discard, "", 0), BuiltinProfile: profile})
			ws.IndexWorkspace()

			writeFiles(t, root, map[string]string{"composer.json": `{"require":{"php":"` + tc.to + `"}}`})
			out := refreshPaths(t, ws, root, "composer.json")
			if out.ProfileChange != GrammarsChanged || len(out.Files) != 0 {
				t.Fatalf("only composer.json changed, and with it the grammars: %+v", out)
			}
			if got := ws.ReparseForGrammars(); got != tc.reparsed {
				t.Errorf("re-read %d files, want %d", got, tc.reparsed)
			}
			for _, fqn := range tc.want {
				if ws.Index.Lookup(fqn) == nil {
					t.Errorf("%s is declared under PHP %s", fqn, tc.to)
				}
			}
			for name := range tc.files {
				uri := symbols.PathToURI(filepath.Join(root, name))
				if tree := ws.Index.GrammarOf(uri).Parse(ws.Index.GetFileSource(uri)); len(tree.Errors) > 0 {
					t.Errorf("%s keeps a tree with syntax errors: %v", name, tree.Errors)
				}
			}
		})
	}
}
