package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkUnknownNamedArgument(b *testing.B) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///lib.php", `<?php
namespace App;
class Repo { public function find(int $id): ?User { return null; } }
class User { public function rename(string $name): void {} public function repo(): Repo { return new Repo(); } }
`)
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Service {\n    public function __construct(private Repo $repo, private User $user) {}\n    public function run(int $id): void {\n")
	for range 200 {
		src.WriteString("        $this->repo->find($id);\n        $this->user->repo()->find($id)?->rename('x');\n")
	}
	src.WriteString("        $this->user->rename(name: 'y');\n    }\n}\n")
	source := src.String()
	file := parser.ParseFile(source)
	types := resolve.NewChainEngine(idx)
	resolver := func(expr, source string, line, col int, file *parser.FileNode) string {
		return types.DiagnosticType(expr, source, protocol.Position{Line: line, Character: col}, file)
	}
	b.ReportAllocs()
	for b.Loop() {
		r := &UnknownNamedArgumentRule{TypeResolver: resolver, Shared: &Shared{}}
		if len(r.Check(file, source, idx)) != 0 {
			b.Fatal("the benchmark file has no unknown named argument")
		}
	}
}
