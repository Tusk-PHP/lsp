package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func BenchmarkInvalidBuilderArgs(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nuse App\\Models\\Category;\nfunction f() {\n")
	for i := 0; i < 200; i++ {
		src.WriteString("    Category::where('name', 1)->select(['id', 'slug'])->with(['products' => fn ($q) => $q])->withSum('products', 'price')->orderBy('created_at')->get();\n")
	}
	src.WriteString("}\n")
	source := src.String()
	file := parser.ParseFile(source, token.Latest)
	rule := setupBuilderArgRule()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rule.Shared = &Shared{}
		if findings := rule.Check(file, source, nil); len(findings) != 0 {
			b.Fatalf("unexpected findings %v", findings)
		}
	}
}
