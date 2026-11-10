package diagnostics

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func TestMemoizeTypeResolverAsksOncePerRun(t *testing.T) {
	calls := 0
	resolve := MemoizeTypeResolver(func(expr, source string, line, col int, file *parser.FileNode) string {
		calls++
		return expr + "@" + source
	})
	for i := 0; i < 3; i++ {
		if got := resolve("$a", "v1", 4, 0, nil); got != "$a@v1" {
			t.Fatalf("got %q", got)
		}
	}
	resolve("$a", "v1", 5, 0, nil) // another line is another question
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	// A new version of the file forgets what the old one said.
	if got := resolve("$a", "v2", 4, 0, nil); got != "$a@v2" || calls != 3 {
		t.Fatalf("got %q after %d calls", got, calls)
	}
	if MemoizeTypeResolver(nil) != nil {
		t.Fatal("nil resolver should stay nil")
	}
}
