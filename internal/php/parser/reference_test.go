package parser

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/phpref"
)

const parseScript = `    try {
        token_get_all($source, TOKEN_PARSE);
        echo $path, "\tok\n";
    } catch (ParseError $e) {
        echo $path, "\t", $e->getLine(), ": ", str_replace("\n", " ", $e->getMessage()), "\n";
    }`

func lineOf(src string, offset uint32) int {
	return strings.Count(src[:offset], "\n") + 1
}

func describe(src string, errs []ast.Error) string {
	if len(errs) == 0 {
		return "ok"
	}
	return fmt.Sprintf("%d: %s", lineOf(src, errs[0].Start), errs[0].Message)
}

func compareWithPHP(t *testing.T, php phpref.PHP, sources map[string]string) {
	t.Helper()
	want := php.Run(t, parseScript, sources)
	var mismatches []string
	for name, src := range sources {
		got := Parse(src, php.Version).Errors
		phpOK := want[name] == "ok"
		if phpOK != (len(got) == 0) {
			mismatches = append(mismatches, fmt.Sprintf("%s\n    PHP: %s\n    us:  %s", name, want[name], describe(src, got)))
		}
	}
	sort.Strings(mismatches)
	if len(mismatches) > 0 {
		t.Errorf("%d of %d sources disagree with PHP %s on syntax errors, first: \n%s", len(mismatches), len(sources), php.Version, strings.Join(mismatches[:min(15, len(mismatches))], "\n"))
	}
}

func TestParserAgreesWithPHPOnTheCorpus(t *testing.T) {
	refs := phpref.Versions(t)
	sources := phpref.ReadAll(t, phpref.Corpus(t))
	for _, php := range refs {
		t.Run(php.Version.String(), func(t *testing.T) { compareWithPHP(t, php, sources) })
	}
}
