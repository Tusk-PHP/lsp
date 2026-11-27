package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkSignatureHelp(b *testing.B) {
	source := signatureHelpBenchSource("format")
	benchSignatureHelp(b, source, source, "format")
}

func BenchmarkSignatureHelpOfAConstruction(b *testing.B) {
	source := signatureHelpBenchSource("new Formatter")
	benchSignatureHelp(b, source, source, "new Formatter")
}

func BenchmarkSignatureHelpAfterAnEditAboveTheCall(b *testing.B) {
	source := signatureHelpBenchSource("format")
	benchSignatureHelp(b, strings.Replace(source, "class Handler", "class Handle", 1), source, "format")
}

func signatureHelpBenchSource(call string) string {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App\\Services;\n\nuse App\\Models\\User;\n\nclass Handler\n{\n")
	for i := range 300 {
		src.WriteString("    public function run")
		src.WriteString(strings.Repeat("x", i%7+1))
		src.WriteString("(User $user): string\n    {\n        $fn = fn ($x) => $x . $user->name();\n")
		if i == 150 {
			src.WriteString("        return " + call + "($fn('a'), \n    }\n\n")
		} else {
			src.WriteString("        return format($fn('a'), 80);\n    }\n\n")
		}
	}
	src.WriteString("}\n")
	return src.String()
}

func benchSignatureHelp(b *testing.B, indexed, source, call string) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///helpers.php", "<?php\nnamespace App\\Services;\nfunction format(string $text, int $width): string {}\nclass Formatter { public function __construct(private string $text, private int $width) {} }\n")
	idx.IndexFile("file:///handler.php", indexed)
	a := NewAnalyzer(idx, nil)
	pos := protocol.Position{Line: 7 + 150*6 + 3, Character: len("        return " + call + "($fn('a'), ")}
	if help := a.GetSignatureHelp("file:///handler.php", source, pos); help == nil || help.ActiveParameter != 1 {
		b.Fatalf("signature help = %+v", help)
	}
	b.ReportAllocs()
	for b.Loop() {
		a.GetSignatureHelp("file:///handler.php", source, pos)
	}
}
