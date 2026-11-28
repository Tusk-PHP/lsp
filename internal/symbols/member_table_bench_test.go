package symbols

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkClassMembersNamed(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Base {\n")
	for i := range 200 {
		fmt.Fprintf(&src, "    public $prop%d;\n    const C%d = 1;\n    public function method%d() {}\n", i, i, i)
	}
	src.WriteString("}\nclass Child extends Base {}\n")
	idx := NewIndex()
	idx.IndexFile("file:///base.php", src.String())
	names := []string{"method7", "Method7", "prop42", "C150", "missing"}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		idx.ClassMembersNamed(`App\Child`, names[i%len(names)])
		i++
	}
}
