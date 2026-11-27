package analyzer

import (
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
}

class Big extends Room {}

class Plain {}

final class Spread {
    public function __construct(string $first, string ...$rest) {}
}

function area(int $width, int $height): int { return 0; }
`

func TestSignatureHelpOfAConstructionShowsTheConstructor(t *testing.T) {
	inHall := "<?php\nnamespace App;\nclass Hall extends Room {\n    public function __construct(int $seats) {}\n    public function make() {\n        return "
	cases := []struct {
		name, src, label string
		active           int
	}{
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			offset := strings.Index(tc.src, "|")
			src := tc.src[:offset] + tc.src[offset+1:]
			idx := symbols.NewIndex()
			idx.RegisterBuiltins()
			idx.IndexFile("file:///app/classes.php", constructorClasses)
			idx.IndexFile("file:///app/call.php", src)
			a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))
			pos := protocol.Position{Line: strings.Count(src[:offset], "\n"), Character: offset - strings.LastIndex(src[:offset], "\n") - 1}
			help := a.GetSignatureHelp("file:///app/call.php", src, pos)
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
