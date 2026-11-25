package analyzer

import (
	"strings"
	"testing"
)

func TestDefinitionOfABareCallIsOnlyAFunction(t *testing.T) {
	room := `<?php
namespace Other;
class Room {
    public function area(): float { return 1.0; }
    public function measure(): float { return area(); }
}
`
	caller := `<?php
namespace App;
use function Lib\scaled as grow;
use function Lib\scaled;
use Lib as L;
$a = area();
$b = helper();
$c = grow(2);
$d = \Lib\scaled(3);
$e = local();
$f = SCALED(4);
$g = l\scaled(5);
`
	helpers := `<?php
function helper() {}
function scaled($n) {}
`
	lib := `<?php
namespace Lib;
function scaled($n) {}
`
	local := `<?php
namespace App;
function local() {}
`
	sources := map[string]string{
		"file:///Room.php":    room,
		"file:///caller.php":  caller,
		"file:///helpers.php": helpers,
		"file:///Lib.php":     lib,
		"file:///local.php":   local,
	}
	a, _ := setupRenameAnalyzer(sources)

	for _, c := range []struct{ uri, needle string }{
		{"file:///caller.php", "area()"},
		{"file:///Room.php", "area(); }"},
	} {
		pos := positionOf(t, sources[c.uri], c.needle)
		if got := a.FindDefinition(c.uri, sources[c.uri], pos); got != nil {
			t.Errorf("definition of the bare call %q in %s: got %v, want none", c.needle, c.uri, got)
		}
		if got := a.FindDefinitionWithManual(c.uri, sources[c.uri], pos, true, ""); got.Location != nil || got.ExternalManualURL != "" {
			t.Errorf("definition with the manual of %q in %s: got %+v, want none", c.needle, c.uri, got)
		}
		if got := a.FindDeclaration(c.uri, sources[c.uri], pos); got != nil {
			t.Errorf("declaration of %q in %s: got %v, want none", c.needle, c.uri, got)
		}
	}

	for needle, wantURI := range map[string]string{
		"helper()":  "file:///helpers.php",
		"grow(2)":   "file:///Lib.php",
		"scaled(3)": "file:///Lib.php",
		"local()":   "file:///local.php",
		"SCALED(4)": "file:///Lib.php",
		"scaled(5)": "file:///Lib.php",
	} {
		pos := positionOf(t, caller, needle)
		if got := a.FindDefinition("file:///caller.php", caller, pos); got == nil || got.URI != wantURI {
			t.Errorf("definition of %q: got %v, want the function in %s", needle, got, wantURI)
		}
	}
}

func BenchmarkFindDefinitionOfABareCall(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Big {\n")
	for range 300 {
		src.WriteString("    public function m(array $a): int { return count($a) + helper(); }\n")
	}
	src.WriteString("}\n$x = helper();\n")
	caller := src.String()
	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///helpers.php": "<?php\nfunction helper() {}\n",
		"file:///caller.php":  caller,
	})
	pos := positionOf(b, caller, "helper();\n")
	b.ReportAllocs()
	for b.Loop() {
		if a.FindDefinition("file:///caller.php", caller, pos) == nil {
			b.Fatal("helper() resolves")
		}
	}
}
