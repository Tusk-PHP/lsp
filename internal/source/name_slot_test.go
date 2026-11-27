package sourcectx

import (
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestNameSlot(t *testing.T) {
	tests := []struct {
		name string
		code string
		ok   bool
		want NameSlot
	}{
		{name: "new", code: "$a = new |", ok: true, want: NameSlot{Kind: NameNew}},
		{name: "new right after the keyword", code: "$a = new|", ok: true, want: NameSlot{Kind: NameNew}},
		{name: "new with a name", code: "$a = new Fo|", ok: true, want: NameSlot{Kind: NameNew, Partial: "Fo"}},
		{name: "new on the next line", code: "return new\n    App\\Fo|", ok: true, want: NameSlot{Kind: NameNew, Partial: `App\Fo`}},
		{name: "new inside a call split over lines", code: "$this->add(\n    new Us|\n);", ok: true, want: NameSlot{Kind: NameNew, Partial: "Us"}},
		{name: "new in the arguments", code: "new Foo(|)"},
		{name: "new before the keyword", code: "$a = n|ew Foo"},
		{name: "new from a variable", code: "new $cla|ss"},
		{name: "anonymous class", code: "new class {|}"},
		{name: "new in a comment", code: "// new Fo|"},
		{name: "use", code: "use |", ok: true, want: NameSlot{Kind: NameUse}},
		{name: "use with a name", code: `use App\Mo|`, ok: true, want: NameSlot{Kind: NameUse, Partial: `App\Mo`}},
		{name: "use function", code: `use function App\he|`, ok: true, want: NameSlot{Kind: NameUse, Partial: `App\he`}},
		{name: "group use split over lines", code: "use App\\{\n    Models\\Us|", ok: true, want: NameSlot{Kind: NameUse, Partial: `App\Models\Us`}},
		{name: "group use after a comma", code: "use App\\{Foo, |};", ok: true, want: NameSlot{Kind: NameUse, Partial: `App\`}},
		{name: "group use right after a comma", code: "use App\\{Foo,|", ok: true, want: NameSlot{Kind: NameUse, Partial: `App\`}},
		{name: "group use prefix", code: "use Ap|\\{Foo};", ok: true, want: NameSlot{Kind: NameUse, Partial: "Ap"}},
		{name: "after the use statement", code: "use App\\Foo;|"},
		{name: "use in a comment", code: "// use App|"},
		{name: "trait use", code: "class A {\n    use Has|\n}", ok: true, want: NameSlot{Kind: NameUse, Partial: "Has"}},
		{name: "trait use after a comma", code: "class A {\n    use HasA, |;\n}", ok: true, want: NameSlot{Kind: NameUse}},
		{name: "trait use adaptations", code: "class A {\n    use HasA { fo| }\n}"},
		{name: "closure use", code: "function () use ($a|) {};"},
		{name: "attribute", code: "#[|", ok: true, want: NameSlot{Kind: NameAttribute}},
		{name: "attribute with a name", code: "#[Ch|", ok: true, want: NameSlot{Kind: NameAttribute, Partial: "Ch"}},
		{name: "second attribute", code: "#[Unique(), Ro|", ok: true, want: NameSlot{Kind: NameAttribute, Partial: "Ro"}},
		{name: "attribute split over lines", code: "#[\n    Ro|\n]\nfunction f() {}", ok: true, want: NameSlot{Kind: NameAttribute, Partial: "Ro"}},
		{name: "attribute on a method", code: "class A {\n    #[Ov|\n    public function f() {}\n}", ok: true, want: NameSlot{Kind: NameAttribute, Partial: "Ov"}},
		{name: "attribute arguments", code: "#[Route('/x|')]\nfunction f() {}"},
		{name: "after the attribute", code: "#[Foo]|\nfunction f() {}"},
		{name: "attribute in a comment", code: "// #[Fo|"},
		{name: "pipe", code: "$a = $x |> ‸", ok: true, want: NameSlot{Kind: NamePipe}},
		{name: "pipe on the next line", code: "$a = $x\n    |>\n    ‸", ok: true, want: NameSlot{Kind: NamePipe}},
		{name: "pipe with a name", code: "$a = $x |> st‸"},
		{name: "before the pipe", code: "$a = $x‸ |> "},
		{name: "or", code: "$a = $x | ‸"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx *CursorContext
			if before, after, ok := strings.Cut(tt.code, "‸"); ok {
				source := "<?php\n" + before
				pos := protocol.Position{Line: strings.Count(source, "\n"), Character: len(source) - strings.LastIndex(source, "\n") - 1}
				ctx = Analyze("file:///a.php", source+after, pos, phpparser.OnlyGrammar(token.Latest))
			} else {
				ctx = analyzeMarked(t, tt.code)
			}
			slot, ok := ctx.NameSlot()
			if ok != tt.ok || slot != tt.want {
				t.Fatalf("got %#v, %v; want %#v, %v", slot, ok, tt.want, tt.ok)
			}
		})
	}
}
