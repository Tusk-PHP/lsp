package checks

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func compileErrorBenchSource() string {
	var src strings.Builder
	src.WriteString("<?php\ndeclare(strict_types=1);\nnamespace App;\n")
	for i := range 300 {
		n := strconv.Itoa(i)
		src.WriteString("final class Service" + n + " {\n    private const LIMIT = 10;\n    public function __construct(private int $id, private array $items = []) {}\n")
		src.WriteString("    public function run(int $n, string ...$names): ?int {\n        foreach ($this->items as $k => $item) {\n            if ($item === null) { continue; }\n            $v = $k > $n ? $item : ($k < 0 ? null : $k);\n            switch ($v) { case 1: break; default: break 2; }\n        }\n")
		src.WriteString("        $f = function (int $x) use ($n): int { return $x + $n; };\n        $g = fn($y) => $f($y, ...$names);\n        return $this->total(limit: self::LIMIT, items: $this->items) ?: null;\n    }\n")
		src.WriteString("    private function total(int $limit, array $items): int { $sum = 0; for ($i = 0; $i < $limit; $i++) { $sum += $items[$i] ?? 0; } return $sum; }\n}\n")
	}
	return src.String()
}

func BenchmarkCompileErrors(b *testing.B) {
	source := compileErrorBenchSource()
	file := parser.ParseFile(source, token.PHP84)
	shared := &Shared{}
	rule := &CompileErrorRule{Shared: shared}
	if got := rule.Check(file, source, nil); len(got) != 0 {
		b.Fatalf("the benchmark file has compile errors: %+v", got)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	for b.Loop() {
		rule.Check(file, source, nil)
	}
}
