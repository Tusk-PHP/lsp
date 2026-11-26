package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type callBenchmark struct {
	index    *symbols.Index
	file     *parser.FileNode
	source   string
	resolver func(expr, source string, line, col int, file *parser.FileNode) string
}

func newCallBenchmark() callBenchmark {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///lib.php", `<?php
namespace App;
class Repo { public function find(int $id): ?User { return null; } }
class User {
    public function rename(string $name): void {}
    public function repo(): Repo { return new Repo(); }
    #[\Deprecated]
    public function retire(): void {}
}
function score(int $a, int $b = 0): int { return $a + $b; }
`)
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Service {\n    public function __construct(private Repo $repo, private User $user) {}\n    public function run(int $id): void {\n")
	for range 200 {
		src.WriteString("        $this->repo->find($id);\n        $this->user->repo()->find($id)?->rename('x');\n        score($id, score(1));\n")
	}
	src.WriteString("        $this->user->rename(name: 'y');\n        $this->user->retire();\n    }\n}\n")
	source := src.String()
	idx.IndexFile("file:///service.php", source)
	types := completion.NewProvider(idx, nil, "")
	return callBenchmark{
		index:  idx,
		file:   parser.ParseFile(source, token.Latest),
		source: source,
		resolver: func(expr, source string, line, col int, file *parser.FileNode) string {
			return types.ResolveDiagnosticType(expr, source, protocol.Position{Line: line, Character: col}, file)
		},
	}
}

func BenchmarkUnknownNamedArgument(b *testing.B) {
	bench := newCallBenchmark()
	b.ReportAllocs()
	for b.Loop() {
		r := &UnknownNamedArgumentRule{TypeResolver: bench.resolver, Shared: &Shared{}}
		if len(r.Check(bench.file, bench.source, bench.index)) != 0 {
			b.Fatal("the benchmark file has no unknown named argument")
		}
	}
}

func BenchmarkArgumentCount(b *testing.B) {
	bench := newCallBenchmark()
	b.ReportAllocs()
	for b.Loop() {
		r := &ArgumentCountRule{TypeResolver: bench.resolver, Shared: &Shared{}}
		if len(r.Check(bench.file, bench.source, bench.index)) != 0 {
			b.Fatal("every call in the benchmark file has the right number of arguments")
		}
	}
}

func BenchmarkDeprecatedUsage(b *testing.B) {
	bench := newCallBenchmark()
	b.ReportAllocs()
	for b.Loop() {
		r := &DeprecatedUsageRule{TypeResolver: bench.resolver, Uses: &MemberUseCache{Shared: &Shared{}}}
		if got := r.Check(bench.file, bench.source, bench.index); len(got) != 1 {
			b.Fatalf("the benchmark file calls one deprecated method, got %d findings", len(got))
		}
	}
}
