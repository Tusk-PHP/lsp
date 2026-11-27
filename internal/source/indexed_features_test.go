package sourcectx_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/hover"
	"github.com/Tusk-PHP/lsp/internal/inlayhint"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const featureClasses = `<?php
namespace App\Models;

class User {
    public string $name = '';
    public function rename(string $to): static { return $this; }
}
`

const featureSource = `<?php
namespace App;

use App\Models\User;

class Room {
    public function __construct(private User $owner, public int $size = 0) {}
    public function area(int $width): int {
        $fn = fn ($x) => $x * $width;
        $user = $this->owner->rename('a');
        return $fn($user->name) + new static($user, 1)->size;
    }
}

function helper(Room $room): int {
    $other = new class(1) extends Room {};
    return $room->area(2);
}
`

const featureURI = "file:///app/room.php"

type features struct {
	idx   *symbols.Index
	a     *analyzer.Analyzer
	hover *hover.Provider
	comp  *completion.Provider
	hints *inlayhint.Provider
	edits *int
}

func newFeatures(indexed ...string) features {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/user.php", featureClasses)
	for _, text := range indexed {
		idx.IndexFile(featureURI, text)
	}
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	return features{idx: idx, a: analyzer.NewAnalyzer(idx, ca), hover: hover.NewProvider(idx, ca, "none"), comp: completion.NewProvider(idx, ca, "none"), hints: inlayhint.NewProvider(idx, ca), edits: new(int)}
}

func (f features) forgetParses(t *testing.T, source string) {
	t.Helper()
	*f.edits++
	for i := range 8 {
		f.idx.IndexFile("file:///app/other"+strconv.Itoa(i)+".php", "<?php\n// "+strconv.Itoa(*f.edits)+"\n")
	}
	if _, _, ok := f.idx.IndexedParse(featureURI, source); ok {
		t.Fatal("the stored parse wasn't forgotten")
	}
}

func (f features) answersAt(source string, pos protocol.Position) []any {
	read := func(string) string { return "" }
	return []any{
		f.hover.GetHover(featureURI, source, pos),
		f.comp.GetCompletions(featureURI, source, pos),
		f.a.FindDefinition(featureURI, source, pos),
		f.a.FindTypeDefinition(featureURI, source, pos),
		f.a.FindAllReferences(featureURI, source, pos, read),
		f.a.GetDocumentHighlights(featureURI, source, pos),
		f.a.PrepareRename(featureURI, source, pos),
		f.a.GetSignatureHelp(featureURI, source, pos),
	}
}

func (f features) documentAnswers(source string) []any {
	cfg := &config.InlayHintsConfig{Enabled: true, VariableTypes: true, ForeachTypes: true, ClosureReturnTypes: true, ReturnTypes: true, ParameterNames: true}
	return []any{f.a.GetDocumentSymbols(featureURI, source), f.hints.GetInlayHints(featureURI, source, cfg)}
}

func featurePositions(src string) []protocol.Position {
	var out []protocol.Position
	for line, text := range strings.Split(src, "\n") {
		for col := 0; col <= len(text); col++ {
			out = append(out, protocol.Position{Line: line, Character: col})
		}
	}
	return out
}

func TestEveryCursorFeatureAnswersAlikeFromTheStoredParse(t *testing.T) {
	edited := strings.Replace(featureSource, "        $fn =", "        $extra = 1;\n        $fn =", 1)
	for _, tc := range []struct {
		name    string
		indexed []string
		source  string
	}{
		{"the text just indexed", []string{featureSource}, featureSource},
		{"an edit the index hasn't read", []string{featureSource}, edited},
		{"an edit indexed again", []string{featureSource, edited}, edited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored, fresh := newFeatures(tc.indexed...), newFeatures(tc.indexed...)
			fresh.forgetParses(t, tc.indexed[len(tc.indexed)-1])
			if got, want := stored.documentAnswers(tc.source), fresh.documentAnswers(tc.source); !reflect.DeepEqual(got, want) {
				t.Errorf("document symbols and inlay hints: %+v, from a fresh parse %+v", got, want)
			}
			answered := make([]int, len(stored.answersAt(tc.source, protocol.Position{})))
			for _, pos := range featurePositions(tc.source) {
				fresh.forgetParses(t, tc.indexed[len(tc.indexed)-1])
				got, want := stored.answersAt(tc.source, pos), fresh.answersAt(tc.source, pos)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("at %v: %+v, from a fresh parse %+v", pos, got, want)
				}
				for i, answer := range got {
					if v := reflect.ValueOf(answer); !v.IsNil() && (v.Kind() != reflect.Slice || v.Len() > 0) {
						answered[i]++
					}
				}
			}
			for i, n := range answered {
				if n == 0 {
					t.Errorf("feature %d answered nowhere in the file", i)
				}
			}
		})
	}
}
