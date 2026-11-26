package sourcectx

import (
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func BenchmarkAnalyze(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App\\Services;\n\nuse App\\Models\\User;\n\nclass Handler\n{\n")
	for i := range 300 {
		src.WriteString("    public function run")
		src.WriteString(strings.Repeat("x", i%7+1))
		src.WriteString("(User $user): string\n    {\n        $fn = fn ($x) => $x . $user->name();\n        return $fn('a');\n    }\n\n")
	}
	src.WriteString("}\n")
	source := src.String()
	pos := protocol.Position{Line: 7 + 150*6 + 2, Character: 30}
	grammars := phpparser.OnlyGrammar(token.Latest)
	b.ReportAllocs()
	for b.Loop() {
		Analyze("file:///handler.php", source, pos, grammars)
	}
}
