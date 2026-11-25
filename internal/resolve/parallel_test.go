package resolve

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

type resolvedFile struct {
	lines []string
	file  *parser.FileNode
	want  string
	read  textpos.Position
}

func manyFiles(n, padding int) (*symbols.Index, []resolvedFile) {
	idx := symbols.NewIndex()
	files := make([]resolvedFile, n)
	for k := range n {
		class := "Class" + strconv.Itoa(k)
		idx.IndexFile("file:///lib/"+class+".php", "<?php\nnamespace App;\nclass "+class+" { public function next(): "+class+" { return $this; } }\n")
		var b strings.Builder
		b.WriteString("<?php\nnamespace App;\nclass Use" + strconv.Itoa(k) + " {\n    public function run(array $items) {\n")
		for p := range padding {
			b.WriteString("        $pad" + strconv.Itoa(p) + " = " + strconv.Itoa(p) + ";\n")
		}
		b.WriteString("        $a = new " + class + "();\n        $b = $a->next();\n        foreach ($items as $i) { $c = $b; }\n        $f = function () use ($c) { return $c; };\n        return $c;\n    }\n}\n")
		source := b.String()
		lines := SplitLines(source)
		files[k] = resolvedFile{lines: lines, file: parser.ParseFile(source), want: `App\` + class, read: textpos.Position{Line: len(lines) - 4, Character: 16}}
	}
	return idx, files
}

func TestParallelResolvesEachFileWithItsOwnScopes(t *testing.T) {
	idx, files := manyFiles(3*maxCachedFiles, 5)
	e := NewEngine(idx)
	const rounds = 4
	got := make([]string, rounds*len(files))
	e.Parallel(len(got), func(r *Resolver, i int) {
		f := files[i%len(files)]
		got[i] = r.ResolveVariableTypeTyped("$c", f.lines, f.read, f.file).FQN
	})
	for i, fqn := range got {
		if want := files[i%len(files)].want; fqn != want {
			t.Errorf("call %d: got %q, want %q", i, fqn, want)
		}
	}
	e.files.mu.Lock()
	cached := len(e.files.files)
	e.files.mu.Unlock()
	if cached > maxCachedFiles {
		t.Errorf("the engine keeps at most %d files, kept %d", maxCachedFiles, cached)
	}
}

func TestOneResolverSwitchingFilesKeepsEachFilesAnswers(t *testing.T) {
	idx, files := manyFiles(2, 3)
	r := NewEngine(idx).Resolver()
	for range 3 {
		for _, f := range files {
			if got := r.ResolveVariableTypeTyped("$c", f.lines, f.read, f.file).FQN; got != f.want {
				t.Errorf("got %q, want %q", got, f.want)
			}
		}
	}
}

func TestParallelCallsEveryIndexOnce(t *testing.T) {
	for _, n := range []int{0, 1, 7, 1000} {
		calls := make([]atomic.Int32, n)
		NewEngine(symbols.NewIndex()).Parallel(n, func(_ *Resolver, i int) { calls[i].Add(1) })
		for i := range calls {
			if c := calls[i].Load(); c != 1 {
				t.Fatalf("n=%d: index %d was called %d times", n, i, c)
			}
		}
	}
}

func TestFileCacheEvictsTheLeastRecentlyUsedFile(t *testing.T) {
	var c fileCache
	keep := c.get("<?php // kept", nil)
	for i := range maxCachedFiles {
		c.get("<?php // "+strconv.Itoa(i), nil)
		if c.get("<?php // kept", nil) != keep {
			t.Fatal("a file in use stays cached")
		}
	}
	if len(c.files) != maxCachedFiles {
		t.Errorf("the cache holds %d files, want %d", len(c.files), maxCachedFiles)
	}
	if _, ok := c.files["<?php // 0"]; ok {
		t.Error("the least recently used file left the cache")
	}
}

func BenchmarkResolveManyFiles(b *testing.B) {
	idx, files := manyFiles(256, 150)
	resolve := func(r *Resolver, i int) {
		f := files[i]
		if got := r.ResolveVariableTypeTyped("$c", f.lines, f.read, f.file).FQN; got != f.want {
			b.Errorf("got %q, want %q", got, f.want)
		}
	}
	b.Run("sequential", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			r := NewEngine(idx).Resolver()
			for i := range files {
				resolve(r, i)
			}
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			NewEngine(idx).Parallel(len(files), resolve)
		}
	})
}
