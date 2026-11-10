package scope

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// reachBenchSource is a method that assigns $r in 1,500 `if` blocks, with a
// read after each, and then in 1,500 `case`s of a switch, with 300 reads after
// it: every read may be reached by more writes than ReachingWrites looks at.
func reachBenchSource() (string, []protocol.Position) {
	var sb strings.Builder
	var reads []protocol.Position
	line := 0
	write := func(s string) {
		sb.WriteString(s)
		sb.WriteByte('\n')
		line++
	}
	write("<?php")
	write("function run(int $n) {")
	write("    $r = new Foo();")
	for i := 0; i < 1500; i++ {
		write("    if ($n > " + strconv.Itoa(i) + ") {")
		write("        $r = new Bar();")
		write("    }")
		reads = append(reads, protocol.Position{Line: line, Character: 4})
		write("    $r->fooOnly();")
	}
	write("    switch ($n) {")
	for i := 0; i < 1500; i++ {
		write("        case " + strconv.Itoa(i) + ":")
		write("            $r = new Bar();")
		write("            break;")
	}
	write("    }")
	for i := 0; i < 300; i++ {
		reads = append(reads, protocol.Position{Line: line, Character: 4})
		write("    $r->fooOnly();")
	}
	write("}")
	return sb.String(), reads
}

// BenchmarkReachingWrites times what each of 1,800 reads of a variable written
// 3,000 times, under conditions, asks for: it's bounded by MaxReachingWrites,
// not by the number of writes.
func BenchmarkReachingWrites(b *testing.B) {
	src, reads := reachBenchSource()
	doc := Collect(src)
	var r *Binding
	for _, cand := range doc.Bindings {
		if cand.Name == "$r" {
			r = cand
		}
	}
	if r == nil || len(r.Rebinds) != 3000 {
		b.Fatal("expected $r with 3,000 rebinds")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, pos := range reads {
			doc.ReachingWrites(r, pos)
		}
	}
}
