package analyzer

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const referencesRoom = `<?php
namespace App;

class Room {
    public float $area = 0.0;

    public function area(): float {
        return $this->area;
    }
}
`

const referencesShape = `<?php
namespace App;

class Shape {
    public float $area = 0.0;

    public function area(): float { return $this->area; }
}
`

const referencesUse = `<?php
namespace App;

function measure(Room $room, Shape $shape): string {
    // area() of the room
    $x = $room->area() + $shape->area() + $room->area + $shape->area;
    return "area: {$room->area()}" . area . $room->area();
}
`

func referencesAt(t *testing.T, uri, needle string) []string {
	t.Helper()
	sources := map[string]string{"file:///Room.php": referencesRoom, "file:///Shape.php": referencesShape, "file:///use.php": referencesUse}
	a, read := setupRenameAnalyzer(sources)
	src := sources[uri]
	var got []string
	for _, l := range a.FindAllReferences(uri, src, positionOf(t, src, needle), read) {
		got = append(got, fmt.Sprintf("%s:%d:%d", strings.TrimPrefix(l.URI, "file:///"), l.Range.Start.Line, l.Range.Start.Character))
	}
	slices.Sort(got)
	return got
}

func TestReferencesOfAMethodAreOnlyItsResolvedUses(t *testing.T) {
	got := referencesAt(t, "file:///use.php", "area() +")
	want := []string{"Room.php:6:20", "use.php:5:16", "use.php:6:51"}
	if !slices.Equal(got, want) {
		t.Errorf("Room::area's declaration and its calls on a Room; never Shape::area, the properties, the comment, the string or the constant:\n got %v\nwant %v", got, want)
	}
}

func TestReferencesOfAPropertyAreOnlyItsDeclarationAndAccesses(t *testing.T) {
	got := referencesAt(t, "file:///Room.php", "$area = 0.0")
	want := []string{"Room.php:4:17", "Room.php:7:22", "use.php:5:49"}
	if !slices.Equal(got, want) {
		t.Errorf("Room::$area's declaration and accesses on a Room, never Shape's:\n got %v\nwant %v", got, want)
	}
}

func TestReferencesOfANameThatResolvesToNothing(t *testing.T) {
	if got := referencesAt(t, "file:///use.php", "area . $room"); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestRenameEditsOnlyTheResolvedUses(t *testing.T) {
	sources := map[string]string{"file:///Room.php": referencesRoom, "file:///Shape.php": referencesShape, "file:///use.php": referencesUse}
	a, read := setupRenameAnalyzer(sources)
	edit := a.Rename("file:///Room.php", referencesRoom, positionOf(t, referencesRoom, "area(): float"), "size", read)
	if edit == nil {
		t.Fatal("Room::area can be renamed")
	}
	count := 0
	for uri, edits := range edit.Changes {
		if uri == "file:///Shape.php" {
			t.Errorf("Shape.php is not edited: %+v", edits)
		}
		count += len(edits)
	}
	if count != 3 {
		t.Errorf("the declaration and the two calls are renamed, got %d edits: %+v", count, edit.Changes)
	}
}

func TestMayMentionNeedsTheNameAsAWord(t *testing.T) {
	method := &symbols.Symbol{Name: "area", Kind: symbols.KindMethod}
	constant := &symbols.Symbol{Name: "AREA", Kind: symbols.KindConstant}
	property := &symbols.Symbol{Name: "$area", Kind: symbols.KindProperty}
	for _, c := range []struct {
		src  string
		sym  *symbols.Symbol
		want bool
	}{
		{"$r->area()", method, true},
		{"$r->areas()", method, false},
		{"$area = 1;", method, false},
		{"$r->size()", method, false},
		{"echo area;", constant, false},
		{"echo AREA;", constant, true},
		{"$r->area", property, true},
		{"public $area;", property, true},
		{"", method, false},
		{"$r->AREA()", method, true},
		{"$r->AREAS()", method, false},
		{"USER::find(1);", &symbols.Symbol{Name: "User", Kind: symbols.KindClass}, true},
		{"echo STRTOLOWER($s);", &symbols.Symbol{Name: "strtolower", Kind: symbols.KindFunction}, true},
		{"echo Area;", constant, false},
		{"$r->AREA", property, false},
		{"return new \\App\\Room();", &symbols.Symbol{Name: "Room", Kind: symbols.KindClass}, true},
		{"use App\\Room;", &symbols.Symbol{Name: "Room", Kind: symbols.KindClass}, true},
		{"use App\\Rooms;", &symbols.Symbol{Name: "Room", Kind: symbols.KindClass}, false},
		{"$r->are", method, false},
		{"class {", &symbols.Symbol{Kind: symbols.KindClass}, false},
		{"$r->area", &symbols.Symbol{Name: "$", Kind: symbols.KindProperty}, false},
	} {
		if got := mayMention(c.src, c.sym); got != c.want {
			t.Errorf("mayMention(%q, %s) = %v, want %v", c.src, c.sym.Name, got, c.want)
		}
	}
}

func TestReferencesAndRenameFollowTheMethodsHierarchy(t *testing.T) {
	sources := map[string]string{
		"file:///Shape.php": "<?php\nnamespace App;\ninterface Shape { public function area(): float; }\n",
		"file:///Room.php":  "<?php\nnamespace App;\nclass Room implements Shape { public function area(): float { return 1.0; } }\nclass Big extends Room { public function area(): float { return 2.0; } }\nclass Circle implements Shape { public function area(): float { return 3.0; } }\nclass Plain { public function area(): float { return 4.0; } }\n",
		"file:///use.php":   "<?php\nnamespace App;\nfunction f(Shape $s, Room $r, Big $b, Plain $p): float {\n    return $s->area() + $r->area() + $b->area() + $p->area();\n}\n",
	}
	a, read := setupRenameAnalyzer(sources)
	pos := positionOf(t, sources["file:///Room.php"], "area(): float { return 1.0")
	var got []string
	for _, l := range a.FindAllReferences("file:///Room.php", sources["file:///Room.php"], pos, read) {
		got = append(got, fmt.Sprintf("%s:%d:%d", strings.TrimPrefix(l.URI, "file:///"), l.Range.Start.Line, l.Range.Start.Character))
	}
	slices.Sort(got)
	want := []string{"Room.php:2:46", "Room.php:3:41", "Room.php:4:48", "Shape.php:2:34", "use.php:3:15", "use.php:3:28", "use.php:3:41"}
	if !slices.Equal(got, want) {
		t.Errorf("references of Room::area are Shape::area, its overrides and implementations and their calls, never Plain::area:\n got %v\nwant %v", got, want)
	}
	edit := a.Rename("file:///Room.php", sources["file:///Room.php"], pos, "size", read)
	if edit == nil {
		t.Fatal("Room::area can be renamed")
	}
	count := 0
	for _, edits := range edit.Changes {
		count += len(edits)
	}
	if count != len(want) {
		t.Errorf("rename edits the whole family and its calls, got %d edits: %+v", count, edit.Changes)
	}
}

func TestRenameRefusesAMethodThatImplementsAVendorInterface(t *testing.T) {
	src := "<?php\nnamespace App;\nclass Bag implements \\Vendor\\Sized { public function size(): int { return 0; } }\n"
	a, read := setupRenameAnalyzer(map[string]string{"file:///Bag.php": src})
	a.index.IndexFileWithSource("file:///vendor/Sized.php", "<?php\nnamespace Vendor;\ninterface Sized { public function size(): int; }\n", symbols.SourceVendor)
	pos := positionOf(t, src, "size(): int")
	if got := a.PrepareRename("file:///Bag.php", src, pos); got != nil {
		t.Errorf("Vendor\\Sized::size can't be renamed, so neither can Bag::size: %+v", got)
	}
	if got := a.Rename("file:///Bag.php", src, pos, "count", read); got != nil {
		t.Errorf("rename of Bag::size: %+v", got)
	}
}

const referencesPlot = `<?php
namespace App;
class Plot {
    public function __construct(private float $area) {
        echo $area;
    }
    public function size(): float { return $this->area; }
    public function local(): float {
        $area = 2.0;
        return $area + $this->area;
    }
}
`

func TestReferencesOfAPromotedPropertyAreTheProperty(t *testing.T) {
	a, read := setupRenameAnalyzer(map[string]string{"file:///Plot.php": referencesPlot})
	var got []string
	for _, l := range a.FindAllReferences("file:///Plot.php", referencesPlot, positionOf(t, referencesPlot, "$area)"), read) {
		got = append(got, fmt.Sprintf("%d:%d", l.Range.Start.Line, l.Range.Start.Character))
	}
	slices.Sort(got)
	if want := []string{"3:46", "6:50", "9:30"}; !slices.Equal(got, want) {
		t.Errorf("the promoted parameter's references are the property's:\n got %v\nwant %v", got, want)
	}
}

func TestRenameOfALocalNamedLikeAPropertyIsTheLocal(t *testing.T) {
	a, read := setupRenameAnalyzer(map[string]string{"file:///Plot.php": referencesPlot})
	pos := positionOf(t, referencesPlot, "$area = 2.0")
	if got := a.PrepareRename("file:///Plot.php", referencesPlot, pos); got == nil || got.Placeholder != "$area" {
		t.Errorf("prepare-rename of the local: %+v", got)
	}
	edit := a.Rename("file:///Plot.php", referencesPlot, pos, "$size", read)
	if edit == nil {
		t.Fatal("the local can be renamed")
	}
	var got []string
	for _, e := range edit.Changes["file:///Plot.php"] {
		got = append(got, fmt.Sprintf("%d:%d", e.Range.Start.Line, e.Range.Start.Character))
	}
	slices.Sort(got)
	if want := []string{"8:8", "9:15"}; !slices.Equal(got, want) || len(edit.Changes) != 1 {
		t.Errorf("only the local and its use are renamed:\n got %v\nwant %v", got, want)
	}
}

func TestReferencesOfAClassIncludeItsPHPDocTypes(t *testing.T) {
	sources := map[string]string{
		"file:///User.php":  "<?php\nnamespace App\\Models;\nclass User {}\n",
		"file:///Other.php": "<?php\nnamespace Other;\nclass User {}\n",
		"file:///doc.php":   "<?php\nnamespace App\\Http;\n/** @var \\App\\Models\\User $u */\n$u = null;\n",
		"file:///use.php": `<?php
namespace App\Http;
use App\Models\User;
use Other\User as Stranger;
class Show {
    /**
     * Shows a User.
     * @param User $user
     * @param Stranger $other
     * @return array<int, \App\Models\User>|\Other\User
     */
    public function __invoke($user, $other) { return []; }
}
`,
	}
	a, read := setupRenameAnalyzer(sources)
	var got []string
	for _, l := range a.FindAllReferences("file:///User.php", sources["file:///User.php"], positionOf(t, sources["file:///User.php"], "User {}"), read) {
		got = append(got, fmt.Sprintf("%s:%d:%d", strings.TrimPrefix(l.URI, "file:///"), l.Range.Start.Line, l.Range.Start.Character))
	}
	slices.Sort(got)
	want := []string{"User.php:2:6", "doc.php:2:21", "use.php:2:15", "use.php:7:14", "use.php:9:38"}
	if !slices.Equal(got, want) {
		t.Errorf("the class's references include the PHPDoc types that resolve to it, not the description or another User:\n got %v\nwant %v", got, want)
	}
	edit := a.Rename("file:///User.php", sources["file:///User.php"], positionOf(t, sources["file:///User.php"], "User {}"), "Member", read)
	if edit == nil || len(edit.Changes["file:///use.php"]) != 3 {
		t.Errorf("rename edits the import and both PHPDoc types: %+v", edit)
	}
}

func TestReferencesIncludeFullyQualifiedNamesAndOtherBladeTemplates(t *testing.T) {
	const view = "file:///views/room.blade.php"
	const quiet = "file:///views/quiet.blade.php"
	templates := map[string]string{
		view:  "<div>{{ \\App\\Room::class }}</div>\n  <span>{{ new \\App\\Room() }}</span>\n",
		quiet: "<p>{{ $title }} in the Room</p>\n",
	}
	a, read := setupRenameAnalyzer(map[string]string{
		"file:///Room.php": referencesRoom,
		"file:///make.php": "<?php\nfunction make() { return new \\App\\Room(); }\n",
	})
	for uri := range templates {
		a.index.AddTemplateFile(uri)
	}
	readAll := func(uri string) string {
		if src, ok := templates[uri]; ok {
			return src
		}
		return read(uri)
	}
	var got []string
	for _, l := range a.FindAllReferences("file:///Room.php", referencesRoom, positionOf(t, referencesRoom, "Room {"), readAll) {
		got = append(got, fmt.Sprintf("%s:%d:%d", strings.TrimPrefix(l.URI, "file:///"), l.Range.Start.Line, l.Range.Start.Character))
	}
	slices.Sort(got)
	want := []string{"Room.php:3:6", "make.php:1:34", "views/room.blade.php:0:13", "views/room.blade.php:1:20"}
	if !slices.Equal(got, want) {
		t.Errorf("references of Room = %v, want %v", got, want)
	}
}

func TestReferencesFromABladeTemplateIncludeItsOwnAndTheOtherFiles(t *testing.T) {
	const view = "file:///views/room.blade.php"
	const other = "file:///views/other.blade.php"
	templates := map[string]string{
		view:  "<div>{{ \\App\\Room::class }}</div>\n",
		other: "<span>{{ new \\App\\Room() }}</span>\n",
	}
	a, read := setupRenameAnalyzer(map[string]string{"file:///Room.php": referencesRoom})
	for uri := range templates {
		a.index.AddTemplateFile(uri)
	}
	readAll := func(uri string) string {
		if src, ok := templates[uri]; ok {
			return src
		}
		return read(uri)
	}
	php := blade.Compile(templates[view]).PHP
	var got []string
	for _, l := range a.FindAllReferences(view, php, positionOf(t, php, "Room::class"), readAll) {
		got = append(got, fmt.Sprintf("%s:%d:%d", strings.TrimPrefix(l.URI, "file:///"), l.Range.Start.Line, l.Range.Start.Character))
	}
	slices.Sort(got)
	roomAt := positionOf(t, php, "Room::class")
	want := []string{"Room.php:3:6", "views/other.blade.php:0:18", fmt.Sprintf("views/room.blade.php:%d:%d", roomAt.Line, roomAt.Character)}
	if !slices.Equal(got, want) {
		t.Errorf("references of Room from a template = %v, want %v", got, want)
	}
}
