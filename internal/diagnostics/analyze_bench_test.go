package diagnostics

import (
	"strconv"
	"strings"
	"testing"
)

func benchmarkSource(methods int, suppress bool) string {
	var b strings.Builder
	b.WriteString("<?php\nnamespace App\\Services;\n\nuse App\\Models\\User;\n\nclass Report\n{\n    private array $rows = [];\n\n")
	for i := 0; i < methods; i++ {
		n := strconv.Itoa(i)
		b.WriteString("    public function total" + n + "(array $items, int $limit = 10): int\n    {\n        $sum = 0;\n")
		b.WriteString("        foreach ($items as $key => $item) {\n            if ($key > $limit) {\n                break;\n            }\n")
		if suppress && i%10 == 0 {
			b.WriteString("            // @tusk-ignore undefined-variable\n")
		}
		b.WriteString("            $sum += $item['amount'] ?? $fallback" + n + ";\n        }\n        $this->rows[] = strlen((string) $sum);\n        return $sum;\n    }\n\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func benchmarkAnalyze(b *testing.B, suppress bool) {
	p := newTestProvider()
	p.index.MarkReady()
	source := benchmarkSource(150, suppress)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Analyze("file:///tmp/Report.php", source)
	}
}

func BenchmarkAnalyze(b *testing.B) { benchmarkAnalyze(b, false) }

func BenchmarkAnalyzeWithSuppressions(b *testing.B) { benchmarkAnalyze(b, true) }
