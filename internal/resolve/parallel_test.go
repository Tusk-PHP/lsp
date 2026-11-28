package resolve

import (
	"runtime"
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
	idx, files := manyFiles(96, 800)
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
	if cached := cachedBytes(&e.files); cached > maxCachedSourceBytes {
		t.Errorf("the engine keeps at most %d bytes of source, kept %d", maxCachedSourceBytes, cached)
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

func cachedBytes(c *fileCache) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := 0
	for source := range c.bySource {
		total += len(source)
	}
	if total != c.bytes {
		panic("the cache miscounts its bytes")
	}
	return total
}

func sourceOfSize(name string, size int) string {
	return "<?php // " + name + "\n" + strings.Repeat("$a = 1;\n", size/8)
}

func TestFileCacheEvictsTheLeastRecentlyUsedFile(t *testing.T) {
	var c fileCache
	kept := sourceOfSize("kept", maxCachedSourceBytes/8)
	keep := c.ofSource(kept)
	var others []string
	for i := range 16 {
		others = append(others, sourceOfSize(strconv.Itoa(i), maxCachedSourceBytes/8))
		c.ofSource(others[i])
		if c.ofSource(kept) != keep {
			t.Fatal("a file in use stays cached")
		}
	}
	if got := cachedBytes(&c); got > maxCachedSourceBytes {
		t.Errorf("the cache holds %d bytes, want at most %d", got, maxCachedSourceBytes)
	}
	if _, ok := c.bySource[others[0]]; ok {
		t.Error("the least recently used file left the cache")
	}
	if _, ok := c.bySource[others[15]]; !ok {
		t.Error("the newest file stays cached")
	}
}

func TestTypingInABigFileKeepsAFewVersions(t *testing.T) {
	e := NewEngine(symbols.NewIndex())
	var last *fileState
	for edit := range 40 {
		source := sourceOfSize("edit "+strconv.Itoa(edit), 242<<10)
		last = e.Resolver().fileOfLines(SplitLines(source))
	}
	if got := cachedBytes(&e.files); got > maxCachedSourceBytes {
		t.Errorf("40 versions of a 242 KB file keep %d bytes of source, want at most %d", got, maxCachedSourceBytes)
	}
	huge := sourceOfSize("huge", 2*maxCachedSourceBytes)
	state := e.Resolver().fileOfSource(huge)
	if e.Resolver().fileOfSource(huge) != state {
		t.Error("a file bigger than the bound stays cached while it is the newest")
	}
	if _, ok := e.files.bySource[last.source]; ok {
		t.Error("older files leave to make room for a file bigger than the bound")
	}
}

func TestNoLinesAreAnEmptyFile(t *testing.T) {
	e := NewEngine(symbols.NewIndex())
	empty := e.Resolver().fileOfLines(nil)
	if empty.source != "" || e.Resolver().fileOfLines([]string{}) != empty {
		t.Error("no lines are the empty file, cached like any other")
	}
}

func TestFreshResolverFindsLinesSplitFromACachedSource(t *testing.T) {
	e := NewEngine(symbols.NewIndex())
	source := sourceOfSize("cached", 64<<10)
	cached := e.Resolver().fileOfSource(source)
	if got := e.Resolver().fileOfLines(strings.Split(source, "\n")); got != cached {
		t.Fatal("lines split again from a cached source find its file")
	}
	edited := strings.Split(source, "\n")
	edited[3] = "$b = 2;"
	got := e.Resolver().fileOfLines(edited)
	if got == cached || got.source != strings.Join(edited, "\n") {
		t.Error("lines that share the first line of a cached file but differ from it are a file of their own")
	}
	if again := e.Resolver().fileOfLines(strings.Split(source, "\n")); again != cached {
		t.Error("the cached file is still found by its lines after a file with the same first line was added")
	}
}

func TestParallelRaisesAWorkersPanicOnTheCaller(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	var calls atomic.Int32
	defer func() {
		p, ok := recover().(*workerPanic)
		if !ok {
			t.Fatal("the panic reaches the caller as a workerPanic")
		}
		if p.value != "boom" || !strings.Contains(p.Error(), "boom") || len(p.stack) == 0 {
			t.Errorf("the panic keeps its value and the worker's stack: %v", p)
		}
		if n := calls.Load(); n == 10000 {
			t.Error("a panic stops the calls not yet started")
		}
	}()
	NewEngine(symbols.NewIndex()).Parallel(10000, func(_ *Resolver, i int) {
		calls.Add(1)
		if i == 10 {
			panic("boom")
		}
	})
	t.Fatal("Parallel returned after a worker panicked")
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
