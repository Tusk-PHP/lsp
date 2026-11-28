package checks

import (
	"os"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// deprecatedSyntaxBenchSource is a long class with closures, try/finally and
// calls, about the size of a large framework file.
func deprecatedSyntaxBenchSource() string {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nuse Foo\\Bar;\nuse function Foo\\helper;\nclass Service {\n    const A = 1, B = 2;\n    public function __construct(private Bar $bar) { $this->boot(fn ($x) => $x * 2); }\n")
	for i := range 300 {
		src.WriteString("    public function run")
		src.WriteString(strings.Repeat("x", i%7+1))
		src.WriteString("(array $items): ?array {\n        try {\n            usort($items, function ($a, $b) { return $a <=> $b; });\n            define('X', [1, 2, 3]);\n        } finally {\n            $this->bar->close();\n        }\n        return array_map(fn ($i) => $i + 1, $items);\n    }\n")
	}
	src.WriteString("}\n")
	return src.String()
}

func BenchmarkDeprecatedSyntax(b *testing.B) {
	source := deprecatedSyntaxBenchSource()
	if path := os.Getenv("TUSK_BENCH_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		source = string(data)
	}
	file := parser.ParseFile(source)
	shared := &Shared{}
	shared.parse(source) // the edit rules share one parse
	b.ReportAllocs()
	for b.Loop() {
		r := &DeprecatedSyntaxRule{PHPVersion: "8.6", Shared: shared}
		if len(r.Check(file, source, nil)) != 0 && os.Getenv("TUSK_BENCH_FILE") == "" {
			b.Fatal("the benchmark file has nothing deprecated")
		}
	}
}

// BenchmarkUnreachableForComparison is another edit rule over the same shared
// tokens, as a yardstick for the one above.
func BenchmarkUnreachableForComparison(b *testing.B) {
	source := deprecatedSyntaxBenchSource()
	file := parser.ParseFile(source)
	shared := &Shared{}
	shared.parse(source)
	b.ReportAllocs()
	for b.Loop() {
		(&UnreachableCodeRule{Shared: shared}).Check(file, source, nil)
	}
}
