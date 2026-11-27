package analyzer

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const constructorClasses = `<?php
namespace Doctrine\ORM\Mapping;

#[\Attribute]
final class Table {
    public function __construct(public ?string $name = null, public ?string $schema = null, public ?array $options = null) {}
}

namespace App;

class Room {
    public function __construct(private readonly string $name, protected int $size = 0, public bool $open = true) {}
    public function rename(string $to, bool $force = false): static { return $this; }
    public static function named(string $name): static { return new static($name); }
}

class Big extends Room {}

class Plain {}

final class Spread {
    public function __construct(string $first, string ...$rest) {}
}

function area(int $width, int $height): int { return 0; }
`

const inHall = "<?php\nnamespace App;\nclass Hall extends Room {\n    public function __construct(int $seats) {}\n    public function make() {\n        return "

type signatureHelpCase struct {
	name, src, label string
	active           int
}

var signatureHelpCases = []signatureHelpCase{
	{"promoted parameters", "<?php\nnamespace App;\nnew Room('a', |", "Room(string $name, int $size, bool $open)", 1},
	{"an inherited constructor", "<?php\nnamespace App;\nnew Big(|", "Big(string $name, int $size, bool $open)", 0},
	{"a class without a constructor", "<?php\nnamespace App;\nnew Plain(|", "Plain()", 0},
	{"a named argument", "<?php\nnamespace App;\nnew Room(open: |", "Room(string $name, int $size, bool $open)", 2},
	{"a named argument after others", "<?php\nnamespace App;\nnew Room('a', open: true, size: |", "Room(string $name, int $size, bool $open)", 1},
	{"a name the variadic takes", "<?php\nnamespace App;\nnew Spread(other: |", "Spread(string $first, string $rest)", 1},
	{"a builtin class", "<?php\nnamespace App;\nnew \\ArrayObject([], |", "ArrayObject(array|object $array, int $flags, string $iteratorClass)", 1},
	{"an attribute through an aliased namespace", "<?php\nnamespace App;\nuse Doctrine\\ORM\\Mapping as ORM;\n#[ORM\\Table(name: 'x', |", "Table(?string $name, ?string $schema, ?array $options)", 1},
	{"an attribute's named argument", "<?php\nnamespace App;\nuse Doctrine\\ORM\\Mapping as ORM;\n#[ORM\\Table(schema: |", "Table(?string $name, ?string $schema, ?array $options)", 1},
	{"new self", inHall + "new self(|", "Hall(int $seats)", 0},
	{"new static", inHall + "new static(|", "Hall(int $seats)", 0},
	{"new parent", inHall + "new parent('a', |", "Room(string $name, int $size, bool $open)", 1},
	{"an anonymous class's own constructor", "<?php\nnamespace App;\n$x = new class(1, |) { public function __construct(int $a, string $b) {} };", "class@anonymous(int $a, string $b)", 1},
	{"an anonymous class's parent constructor", "<?php\nnamespace App;\n$x = new class('a', |) extends Room {};", "class@anonymous(string $name, int $size, bool $open)", 1},
	{"a function's named argument", "<?php\nnamespace App;\narea(height: |", "area(int $width, int $height): int", 1},
	{"a method call on a variable", "<?php\nnamespace App;\n$room = new Room('a');\n$room->rename('b', |", "rename(string $to, bool $force): static", 1},
	{"a static method call", "<?php\nnamespace App;\nRoom::named(|", "named(string $name): static", 0},
	{"a method call on $this", inHall + "$this->rename(force: |", "rename(string $to, bool $force): static", 1},
}

func signatureHelpAt(t *testing.T, src string, indexed ...string) (*protocol.SignatureHelp, string, protocol.Position) {
	t.Helper()
	offset := strings.Index(src, "|")
	src = src[:offset] + src[offset+1:]
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/classes.php", constructorClasses)
	for _, text := range indexed {
		idx.IndexFile("file:///app/call.php", strings.Replace(text, "|", "", 1))
	}
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))
	pos := protocol.Position{Line: strings.Count(src[:offset], "\n"), Character: offset - strings.LastIndex(src[:offset], "\n") - 1}
	return a.GetSignatureHelp("file:///app/call.php", src, pos), src, pos
}

func TestSignatureHelpOfAConstructionShowsTheConstructor(t *testing.T) {
	for _, tc := range signatureHelpCases {
		t.Run(tc.name, func(t *testing.T) {
			help, _, _ := signatureHelpAt(t, tc.src, tc.src)
			if help == nil || len(help.Signatures) != 1 {
				t.Fatalf("signature help = %+v, want %s", help, tc.label)
			}
			sig := help.Signatures[0]
			if sig.Label != tc.label || help.ActiveParameter != tc.active {
				t.Errorf("got %s with parameter %d active, want %s with %d", sig.Label, help.ActiveParameter, tc.label, tc.active)
			}
			for _, p := range sig.Parameters {
				if !strings.Contains(sig.Label, p.Label) {
					t.Errorf("parameter %q isn't in the label %q", p.Label, sig.Label)
				}
			}
		})
	}
}

func TestSignatureHelpOfAnAnonymousClassNotYetIndexedIsQuiet(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/classes.php", constructorClasses)
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))
	src := "<?php\nnamespace App;\n$x = new class(1, ) extends Room {};"
	pos := protocol.Position{Line: 2, Character: len("$x = new class(1, ")}
	if help := a.GetSignatureHelp("file:///app/call.php", src, pos); help != nil {
		t.Errorf("signature help = %+v, want none for a class the index hasn't read", help)
	}
}

func TestSignatureHelpAnswersAlikeWithAndWithoutTheIndexedParse(t *testing.T) {
	for _, tc := range signatureHelpCases {
		t.Run(tc.name, func(t *testing.T) {
			reused, src, pos := signatureHelpAt(t, tc.src, tc.src)
			idx := symbols.NewIndex()
			idx.RegisterBuiltins()
			idx.IndexFile("file:///app/classes.php", constructorClasses)
			idx.IndexFile("file:///app/call.php", src)
			for i := range 8 {
				idx.IndexFile("file:///app/other"+strings.Repeat("x", i)+".php", "<?php\n")
			}
			if _, _, ok := idx.IndexedParse("file:///app/call.php", src); ok {
				t.Fatal("the call's parse is still kept")
			}
			parsed := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none")).GetSignatureHelp("file:///app/call.php", src, pos)
			if !reflect.DeepEqual(reused, parsed) {
				t.Errorf("from the indexed parse %+v, from a fresh parse %+v", reused, parsed)
			}
		})
	}
}

func TestSignatureHelpAfterAnEditAnswersFromTheEditedText(t *testing.T) {
	for _, tc := range signatureHelpCases {
		if strings.Contains(tc.name, "anonymous") {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			before := strings.Replace(tc.src, "|", "", 1)
			edited := strings.Replace(tc.src, "namespace App;\n", "namespace App;\n\nuse Other\\Thing;\n", 1)
			for _, indexed := range [][]string{{before}, {before, edited}} {
				help, _, _ := signatureHelpAt(t, edited, indexed...)
				if help == nil || help.Signatures[0].Label != tc.label || help.ActiveParameter != tc.active {
					t.Errorf("indexed %d versions: %+v, want %s with %d", len(indexed), help, tc.label, tc.active)
				}
			}
		})
	}
}
