package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkSignatureHelp(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App\\Services;\n\nuse App\\Models\\User;\n\nclass Handler\n{\n")
	for i := range 300 {
		src.WriteString("    public function run")
		src.WriteString(strings.Repeat("x", i%7+1))
		src.WriteString("(User $user): string\n    {\n        $fn = fn ($x) => $x . $user->name();\n")
		if i == 150 {
			src.WriteString("        return format($fn('a'), \n    }\n\n")
		} else {
			src.WriteString("        return format($fn('a'), 80);\n    }\n\n")
		}
	}
	src.WriteString("}\n")
	source := src.String()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///helpers.php", "<?php\nnamespace App\\Services;\nfunction format(string $text, int $width): string {}\n")
	idx.IndexFile("file:///handler.php", source)
	a := NewAnalyzer(idx, nil)
	pos := protocol.Position{Line: 7 + 150*6 + 3, Character: len("        return format($fn('a'), ")}
	if help := a.GetSignatureHelp("file:///handler.php", source, pos); help == nil || help.ActiveParameter != 1 {
		b.Fatalf("signature help = %+v", help)
	}
	b.ReportAllocs()
	for b.Loop() {
		a.GetSignatureHelp("file:///handler.php", source, pos)
	}
}
