package scope

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func scopeAtByScan(d *Document, pos textpos.Position) *Scope {
	var best *Scope
	for _, scope := range d.Scopes {
		if !containsPos(scope.Range, pos) {
			continue
		}
		if best == nil || scopeContains(best.Range, scope.Range) {
			best = scope
		}
	}
	return best
}

func TestScopeAtIsTheInnermostScopeAsAScanFindsIt(t *testing.T) {
	sources := map[string]string{"nested": `<?php
function outer($a) {
    $f = function ($b) use ($a) {
        return fn ($c) => $a + $b + $c;
    };
    $g = fn ($d) => fn ($e) => $d + $e; $h = function () {};
}
class Room {
    public function area() { return array_map(fn ($x) => $x * 2, [1]); }
}
$anon = new class { public function run() { return static fn () => 1; } };
`}
	for _, root := range []string{"../../testdata/project", "../../testdata/php-features", "../../testdata/laravel/app"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".php") {
				return err
			}
			src, err := os.ReadFile(path)
			sources[path] = string(src)
			return err
		})
		if err != nil {
			t.Fatalf("reading %s: %v", root, err)
		}
	}
	if len(sources) < 10 {
		t.Fatalf("only %d sources to compare", len(sources))
	}
	for name, src := range sources {
		doc := Collect(src)
		lines := strings.Split(src, "\n")
		for line := -1; line <= len(lines); line++ {
			width := 0
			if line >= 0 && line < len(lines) {
				width = len(lines[line])
			}
			for col := 0; col <= width+1; col++ {
				pos := textpos.Position{Line: line, Character: col}
				if got, want := doc.ScopeAt(pos), scopeAtByScan(doc, pos); got != want {
					t.Fatalf("%s at %v: scope %+v, a scan of every scope finds %+v", name, pos, got, want)
				}
			}
		}
	}
}
