package completion

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// benchProvider indexes 5,000 classes over 50 namespaces and a file with a
// 200-line method, so completion runs against a project-sized index.
func benchProvider(b *testing.B) (*Provider, string, []string) {
	b.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for n := 0; n < 50; n++ {
		var src strings.Builder
		ns := "App\\Domain" + strconv.Itoa(n)
		src.WriteString("<?php\nnamespace " + ns + ";\n")
		for c := 0; c < 100; c++ {
			name := "User" + strconv.Itoa(n) + "x" + strconv.Itoa(c)
			src.WriteString("class " + name + " { public function save() {} public function load(int $id) {} public $name; }\n")
		}
		idx.IndexFileWithSource("file:///n"+strconv.Itoa(n)+".php", src.String(), symbols.SourceProject)
	}
	var body strings.Builder
	body.WriteString("<?php\nnamespace App\\Domain1;\n\nclass Service\n{\n    public function run(array $rows, int $limit)\n    {\n")
	for i := 0; i < 200; i++ {
		body.WriteString("        $v" + strconv.Itoa(i) + " = $rows[" + strconv.Itoa(i) + "] ?? $limit;\n")
	}
	body.WriteString("        $user = new User1x1();\n        CURSOR\n    }\n}\n")
	return NewProvider(idx, nil, ""), body.String(), nil
}

func benchComplete(b *testing.B, typed string) {
	p, src, _ := benchProvider(b)
	line := strings.Count(src[:strings.Index(src, "CURSOR")], "\n")
	source := strings.Replace(src, "CURSOR", typed, 1)
	pos := protocol.Position{Line: line, Character: 8 + len(typed)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(p.GetCompletions("file:///s.php", source, pos)) == 0 {
			b.Fatal("no completions")
		}
	}
}

func BenchmarkCompleteNames(b *testing.B)     { benchComplete(b, "User1") }
func BenchmarkCompleteMembers(b *testing.B)   { benchComplete(b, "$user->") }
func BenchmarkCompleteVariables(b *testing.B) { benchComplete(b, "$v1") }
