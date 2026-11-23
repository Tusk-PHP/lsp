package rule

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var documentedRuleRow = regexp.MustCompile("^\\| `([a-z-]+)` \\|")

func TestDiagnosticsPageListsEveryRule(t *testing.T) {
	page, err := os.ReadFile("../../docs/diagnostics.md")
	if err != nil {
		t.Fatal(err)
	}
	_, table, ok := strings.Cut(string(page), "### Built-in rules\n")
	if !ok {
		t.Fatal("docs/diagnostics.md has no Built-in rules section")
	}
	table, _, _ = strings.Cut(table, "\n#")
	var documented []Code
	for _, line := range strings.Split(table, "\n") {
		if m := documentedRuleRow.FindStringSubmatch(line); m != nil {
			documented = append(documented, Code(m[1]))
		}
	}
	for _, code := range All {
		if !slices.Contains(documented, code) {
			t.Errorf("%q is a rule but docs/diagnostics.md doesn't list it", code)
		}
	}
	for _, code := range documented {
		if !slices.Contains(All, code) {
			t.Errorf("docs/diagnostics.md lists %q, which isn't a rule", code)
		}
	}
}
