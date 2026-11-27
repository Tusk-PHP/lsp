package completion

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkArrayKeysFromAMethodCall(b *testing.B) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///repo.php", shapeLibrary["file:///repo.php"])
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Controller {\n")
	for i := range 300 {
		fmt.Fprintf(&src, "    public function m%d(Repo $repo): int { return count($repo->load()); }\n", i)
	}
	src.WriteString("    public function show(Repo $repo) {\n        $r = $repo->load();\n        $r['")
	source := src.String()
	pos := protocol.Position{Line: strings.Count(source, "\n"), Character: len("        $r['")}
	p := NewProvider(idx, nil, "")
	b.ReportAllocs()
	for b.Loop() {
		p.GetCompletions("file:///app/use.php", source, pos)
	}
}

func BenchmarkArrayKeysOnAReceiver(b *testing.B) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/Post.php", shapeReceivers)
	p := NewProvider(idx, nil, "")
	for _, receiver := range []string{"$p->meta", "Post::$stats", "$p->summary()", "$p->meta['author']"} {
		var src strings.Builder
		src.WriteString("<?php\nnamespace App;\nclass Controller {\n")
		for i := range 300 {
			fmt.Fprintf(&src, "    public function m%d(Post $p): int { return count($p->summary()); }\n", i)
		}
		cursorLine := "        " + receiver + "['"
		src.WriteString("    public function show(Post $p) {\n" + cursorLine)
		source := src.String()
		pos := protocol.Position{Line: strings.Count(source, "\n"), Character: len(cursorLine)}
		if len(p.GetCompletions("file:///app/use.php", source, pos)) == 0 {
			b.Fatalf("%s[' offers no keys", receiver)
		}
		b.Run(receiver, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p.GetCompletions("file:///app/use.php", source, pos)
			}
		})
	}
}
