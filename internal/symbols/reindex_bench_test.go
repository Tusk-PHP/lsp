package symbols

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkReindexFileInLargeNamespace(b *testing.B) {
	var other strings.Builder
	other.WriteString("<?php\nnamespace App;\n")
	for i := range 5000 {
		fmt.Fprintf(&other, "function f%d() {}\n", i)
	}
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\n")
	for i := range 500 {
		fmt.Fprintf(&src, "const C%d = %d;\nfunction g%d() {}\n", i, i, i)
	}
	texts := [2]string{src.String(), src.String() + "\n"}
	idx := NewIndex()
	idx.IndexFile("file:///other.php", other.String())
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		idx.IndexFile("file:///reindexed.php", texts[i%2])
		i++
	}
}
