package analyzer

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

const highlightRooms = `<?php
namespace App;

class Room {
    private float $area = 0.0;

    public function area(): float {
        return $this->area;
    }

    public function describe(Shape $shape): string {
        $area = $shape->area();
        // area() is cached
        $label = "area: {$this->area}";
        return $this->area() . f(area: 1) . $area;
    }
}

class Shape {
    public function area(): float { return 0.0; }
}

function measure(Room $room, Shape $shape): float {
    return $room->area() + $shape->area() + helper();
}

function helper(): float { return 0.0; }
$x = \App\helper() + helper() + Room::class;
$y = unknown() + nothing;
`

type highlightAt struct {
	line, char int
	kind       protocol.DocumentHighlightKind
}

func highlightsOf(t *testing.T, needle string, nth int) []highlightAt {
	t.Helper()
	a, _ := setupRenameAnalyzer(map[string]string{"file:///rooms.php": highlightRooms})
	offset := -1
	for range nth + 1 {
		next := strings.Index(highlightRooms[offset+1:], needle)
		if next < 0 {
			t.Fatalf("%q #%d isn't in the source", needle, nth)
		}
		offset += next + 1
	}
	pos := protocol.Position{Line: strings.Count(highlightRooms[:offset], "\n"), Character: offset - strings.LastIndex(highlightRooms[:offset], "\n") - 1}
	var got []highlightAt
	for _, h := range a.GetDocumentHighlights("file:///rooms.php", highlightRooms, pos) {
		got = append(got, highlightAt{h.Range.Start.Line, h.Range.Start.Character, h.Kind})
	}
	return got
}

const (
	write = protocol.DocumentHighlightKindWrite
	read  = protocol.DocumentHighlightKindRead
	text  = protocol.DocumentHighlightKindText
)

func TestHighlightsOfAMethodAreOnlyThatMethod(t *testing.T) {
	got := highlightsOf(t, "area()", 0)
	want := []highlightAt{{6, 20, write}, {14, 22, read}, {23, 18, read}}
	if !slices.Equal(got, want) {
		t.Errorf("the method's declaration and its calls on a Room only, never Shape::area, the property, the local, a comment, a string or a named argument:\n got %v\nwant %v", got, want)
	}
	if got := highlightsOf(t, "area();\n        //", 0); !slices.Equal(got, []highlightAt{{11, 24, text}, {19, 20, write}, {23, 35, read}}) {
		t.Errorf("from a call on $shape the target is Shape::area: %v", got)
	}
}

func TestHighlightsOfAPropertyAreItsDeclarationAndAccesses(t *testing.T) {
	got := highlightsOf(t, "$area = 0.0", 0)
	want := []highlightAt{{4, 18, write}, {7, 22, read}, {13, 32, read}}
	if !slices.Equal(got, want) {
		t.Errorf("the property's declaration and $this->area, in code and in the string, never the local $area or the method:\n got %v\nwant %v", got, want)
	}
}

func TestHighlightsOfAFunctionAndAClass(t *testing.T) {
	if got := highlightsOf(t, "helper();\n}", 0); !slices.Equal(got, []highlightAt{{23, 44, text}, {26, 9, write}, {27, 10, read}, {27, 21, read}}) {
		t.Errorf("function: %v", got)
	}
	if got := highlightsOf(t, "Room::class", 0); !slices.Equal(got, []highlightAt{{3, 6, write}, {22, 17, read}, {27, 32, text}}) {
		t.Errorf("class: the declaration, the parameter type and Room::class, got %v", got)
	}
}

func TestHighlightsOfANameThatResolvesToNothing(t *testing.T) {
	for _, needle := range []string{"unknown()", "nothing;"} {
		if got := highlightsOf(t, needle, 0); len(got) != 0 {
			t.Errorf("%s: got %v, want none", needle, got)
		}
	}
}

func (h highlightAt) String() string { return fmt.Sprintf("%d:%d/%d", h.line, h.char, h.kind) }

func TestHighlightsOfALocalNamedLikeAPropertyAreTheLocal(t *testing.T) {
	got := highlightsOf(t, "$area = $shape", 0)
	want := []highlightAt{{11, 8, write}, {14, 44, read}}
	if !slices.Equal(got, want) {
		t.Errorf("the local $area and its use, never the property:\n got %v\nwant %v", got, want)
	}
}

func TestHighlightsOfAPromotedPropertyAreThePropertyAndTheParameter(t *testing.T) {
	src := `<?php
namespace App;
class Plot {
    public function __construct(private float $area) {
        echo $area;
    }
    public function size(): float { return $this->area; }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///plot.php": src})
	var got []highlightAt
	for _, h := range a.GetDocumentHighlights("file:///plot.php", src, positionOf(t, src, "$area)")) {
		got = append(got, highlightAt{h.Range.Start.Line, h.Range.Start.Character, h.Kind})
	}
	if want := []highlightAt{{3, 46, write}, {4, 13, read}, {6, 50, read}}; !slices.Equal(got, want) {
		t.Errorf("the promoted parameter is the property's declaration, highlighted with the parameter's use and $this->area:\n got %v\nwant %v", got, want)
	}
}

func BenchmarkDocumentHighlights(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Shape { public function area(): float { return 0.0; } }\nclass Room {\n    public function area(): float { return 1.0; }\n")
	for i := range 300 {
		fmt.Fprintf(&src, "    public function m%d(Shape $s): float {\n        // area() here\n        return $this->area() + $s->area() + strlen('area');\n    }\n", i)
	}
	src.WriteString("}\n")
	source := src.String()
	a, _ := setupRenameAnalyzer(map[string]string{"file:///big.php": source})
	pos := protocol.Position{Line: 4, Character: 21}
	b.ReportAllocs()
	for b.Loop() {
		if len(a.GetDocumentHighlights("file:///big.php", source, pos)) == 0 {
			b.Fatal("the method is highlighted")
		}
	}
}

func TestHighlightsNeedANameUnderTheCursor(t *testing.T) {
	a, _ := setupRenameAnalyzer(map[string]string{"file:///rooms.php": highlightRooms})
	for _, pos := range []protocol.Position{{Line: 2, Character: 0}, {Line: 500, Character: 0}} {
		if got := a.GetDocumentHighlights("file:///rooms.php", highlightRooms, pos); got != nil {
			t.Errorf("at %v there is no name to highlight, got %v", pos, got)
		}
	}
}
