package scope

import (
	"strconv"
	"strings"
	"testing"
)

// BenchmarkCollect collects the scopes of a 2,000-line class whose methods
// assign, branch, loop and reassign their parameters.
func BenchmarkCollect(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("<?php\nnamespace App;\n\nclass Big\n{\n")
	for i := 0; i < 200; i++ {
		n := strconv.Itoa(i)
		sb.WriteString("    public function m" + n + "(Foo $a, array $items, bool $c)\n    {\n")
		sb.WriteString("        $total = 0;\n        $a = $a->normalize();\n")
		sb.WriteString("        if ($c) {\n            $a = new Bar();\n        } else $total = 1;\n")
		sb.WriteString("        foreach ($items as $key => $item) {\n            $total += $item->price;\n        }\n")
		sb.WriteString("        return $total;\n    }\n\n")
	}
	sb.WriteString("}\n")
	src := sb.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Collect(src)
	}
}
