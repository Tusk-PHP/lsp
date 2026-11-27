package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestValuePreviewCutsAtATokenBoundary(t *testing.T) {
	long := strings.Repeat("self::FLAG|", 20) + "self::LAST"
	for _, tc := range []struct{ value, want string }{
		{"[1,2]", "[1,2]"},
		{long, strings.Repeat("self::FLAG|", 14) + "self::…"},
		{"'" + strings.Repeat("x", 200) + "'", "…"},
		{"f(" + strings.Repeat("\"{$a['k']}\",", 20) + ")", "f(" + strings.Repeat("\"{$a['k']}\",", 13) + "…)"},
		{"[[" + strings.Repeat("1,", 100) + "]]", "[[" + strings.Repeat("1,", 79) + "…]]"},
		{"f(" + strings.Repeat("1,", 100) + ")", "f(" + strings.Repeat("1,", 79) + "…)"},
		{"new class { " + strings.Repeat("const A = 1; ", 20) + "}", "new class { " + strings.Repeat("const A = 1; ", 11) + "const…}"},
		{"<<<EOT\n" + strings.Repeat("a, ", 60) + "\nEOT", "…"},
		{"[<<<EOT\na, b\nEOT, " + strings.Repeat("1, ", 60) + "]", "[<<<EOT\na, b\nEOT, " + strings.Repeat("1, ", 46) + "1,…]"},
	} {
		if got := valuePreview(tc.value); got != tc.want {
			t.Errorf("valuePreview(%q)\n got %q\nwant %q", tc.value, got, tc.want)
		}
	}
}

func TestEnumCaseHoverShowsItsValueBounded(t *testing.T) {
	p := NewProvider(symbols.NewIndex(), nil, "none")
	for _, tc := range []struct{ value, want string }{
		{"'hearts'", "case Hearts = 'hearts'"},
		{"'" + strings.Repeat("h", 200) + "'", "case Hearts = …"},
	} {
		decl := p.formatHoverDeclaration(&symbols.Symbol{Name: "Hearts", FQN: `App\Suit::Hearts`, Kind: symbols.KindEnumCase, ParentFQN: `App\Suit`, Value: tc.value}, nil)
		if !strings.Contains(decl, tc.want) {
			t.Errorf("enum case with value %.20q: declaration %q, want %q", tc.value, decl, tc.want)
		}
	}
}
