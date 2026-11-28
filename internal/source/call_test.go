package sourcectx

import (
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestFunctionCallMarksOnlyANameCalledAsAFunction(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"area|(1);", true},
		{"$x = ar|ea (1);", true},
		{"\\App\\ar|ea(1);", true},
		{"App\\ar|ea(1);", true},
		{"echo area|;", false},
		{"new Ar|ea(1);", false},
		{"new \\App\\Ar|ea(1);", false},
		{"function ar|ea() {}", false},
		{"function &ar|ea() {}", false},
		{"public function &ar|ea() {}", false},
		{"$x = $a & ar|ea(1);", true},
		{"$this->ar|ea();", false},
		{"Room::ar|ea();", false},
		{"#[Ar|ea(1)]\nfunction f() {}", false},
		{"#[Other, Ar|ea([1, 2])]\nfunction f() {}", false},
		{"$a = [ar|ea(1)];", true},
		{"f(ar|ea: 1);", false},
		{"/** ar|ea() */", false},
		{"$x = [f(1)[0], ar|ea(2)];", true},
		{"if (true) { ar|ea(1); }", true},
		{"#[Other([1], f(2)), Ar|ea(1)]\nfunction f() {}", false},
		{"$ar|ea(1);", false},
		{"echo ar|ea", false},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			offset := strings.Index(src, "|")
			src = src[:offset] + src[offset+1:]
			line := strings.Count(src[:offset], "\n")
			char := offset - strings.LastIndex(src[:offset], "\n") - 1
			ctx := Analyze("file:///a.php", src, protocol.Position{Line: line, Character: char}, phpparser.OnlyGrammar(token.Latest))
			if ctx == nil {
				t.Fatal("expected cursor context")
			}
			if ctx.FunctionCall != tc.want {
				t.Fatalf("FunctionCall = %v, want %v (word %q)", ctx.FunctionCall, tc.want, ctx.SymbolText)
			}
		})
	}
}

func TestCallAtFindsTheCalleeAndArgumentFromTokens(t *testing.T) {
	cases := []struct {
		src      string
		ok       bool
		callee   string
		argument int
		function bool
	}{
		{"area(|", true, "area", 0, true},
		{"area($a, 'x, y', |", true, "area", 2, true},
		{"area(f(1, 2), |", true, "area", 1, true},
		{"area([1, 2, |", true, "area", 0, true},
		{"area(\n    $a,\n    |", true, "area", 1, true},
		{"$r->area($a, /* , */ |", true, "area", 1, false},
		{"area(1); |", false, "", 0, false},
		{"function () { |", false, "", 0, false},
		{"area(function () { $x = [1]; }, |", true, "area", 1, true},
		{"$fn(|", false, "", 0, false},
		{"function &area(|", true, "area", 0, false},
		{"f(); area(|", true, "area", 0, true},
		{"#[Other([1]), Area(|", true, "Area", 0, false},
		{"#[A] fn() => area(|", true, "area", 0, true},
		{"|", false, "", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			offset := strings.Index(src, "|")
			src = src[:offset] + src[offset+1:]
			pos := protocol.Position{Line: strings.Count(src[:offset], "\n"), Character: offset - strings.LastIndex(src[:offset], "\n") - 1}
			call, ok := CallAt(symbols.NewIndex(), "file:///a.php", src, pos)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if call.Name != tc.callee || call.Argument != tc.argument || (call.Callee == CalleeFunction) != tc.function {
				t.Errorf("callee %q, argument %d; want %q, %d", call.Name, call.Argument, tc.callee, tc.argument)
			}
		})
	}
}

func TestCalledMarksAnyNameFollowedByItsArguments(t *testing.T) {
	for src, want := range map[string]bool{
		"$this->ar|ea();":             true,
		"$this->ar|ea;":               false,
		"Room::ar|ea(1);":             true,
		"ar|ea(1);":                   true,
		"new Ar|ea(1);":               false,
		"function ar|ea() {}":         false,
		"function &ar|ea() {}":        false,
		"public function &ar|ea() {}": false,
		"$a & ar|ea(1);":              true,
	} {
		full := "<?php\n" + src + "\n"
		offset := strings.Index(full, "|")
		full = full[:offset] + full[offset+1:]
		pos := protocol.Position{Line: strings.Count(full[:offset], "\n"), Character: offset - strings.LastIndex(full[:offset], "\n") - 1}
		if got := Analyze("file:///a.php", full, pos, phpparser.OnlyGrammar(token.Latest)).Called; got != want {
			t.Errorf("%s: Called = %v, want %v", src, got, want)
		}
	}
}

func TestMemberAccessReadsTheFormOfTheAccess(t *testing.T) {
	for src, want := range map[string]resolve.MemberAccess{
		"Room::SI|ZE;":    resolve.ConstantAccess,
		"Room::$SI|ZE;":   resolve.PropertyAccess,
		"Room::si|ze();":  resolve.CallAccess,
		"$room->si|ze;":   resolve.PropertyAccess,
		"$room->si|ze();": resolve.CallAccess,
		"$room?->si|ze;":  resolve.PropertyAccess,
	} {
		full := "<?php\n" + src + "\n"
		offset := strings.Index(full, "|")
		full = full[:offset] + full[offset+1:]
		pos := protocol.Position{Line: strings.Count(full[:offset], "\n"), Character: offset - strings.LastIndex(full[:offset], "\n") - 1}
		if got := Analyze("file:///a.php", full, pos, phpparser.OnlyGrammar(token.Latest)).MemberAccess(); got != want {
			t.Errorf("%s: MemberAccess = %v, want %v", src, got, want)
		}
	}
}

func TestCallAtFromADeclarationLexesOnlyFromThere(t *testing.T) {
	cases := []struct {
		name, src, from string
		callee          string
		argument        int
		function        bool
		unsure          bool
	}{
		{"a call after the declaration", "class A { function run() { area(1, |", "run", "area", 1, true, false},
		{"a member call after the declaration", "class A { function run() { $this->area(|", "run", "area", 0, false, false},
		{"the declaration's own parameters", "class A { function run($a, |", "run", "run", 1, false, false},
		{"a call around the declaration", "area(new class { function run() {} }, |", "run", "area", 1, true, false},
		{"an attribute in the declaration's parameters", "function run(#[Room(|", "run", "Room", 0, false, false},
		{"a call in a parameter's default value", "function run($a = area(|", "run", "area", 0, false, true},
		{"the declaration's name called on the next line", "f(run\n(|", "run", "run", 0, false, true},
		{"an anonymous class's own arguments", "$x = new class(1, |", "class", "class", 1, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			offset := strings.Index(src, "|")
			src = src[:offset] + src[offset+1:]
			call, _, ok := callBefore(src[:offset], uint32(strings.Index(src, tc.from)))
			if !ok {
				t.Fatal("no call")
			}
			if call.Name != tc.callee || call.Argument != tc.argument || (call.Callee == CalleeFunction) != tc.function {
				t.Errorf("got %+v, want %q argument %d function %v", call, tc.callee, tc.argument, tc.function)
			}
			whole, _, _ := callBefore(src[:offset], 0)
			if tc.unsure {
				whole.Callee = CalleeName
			}
			if whole.Name != call.Name || whole.Offset != call.Offset || whole.Argument != call.Argument || whole.Callee != call.Callee {
				t.Errorf("lexing the whole file gives %+v, from the declaration %+v", whole, call)
			}
		})
	}
}

func TestCallAtOutsideTheSource(t *testing.T) {
	idx := symbols.NewIndex()
	for _, pos := range []protocol.Position{{Line: -1}, {Line: 5}} {
		if _, ok := CallAt(idx, "file:///a.php", "<?php\nf(", pos); ok {
			t.Errorf("%v: a call outside the source", pos)
		}
	}
	for _, src := range []string{"<?php\n(", "(", "f"} {
		if _, ok := CallAt(idx, "file:///a.php", src, protocol.Position{Line: strings.Count(src, "\n"), Character: 1}); ok {
			t.Errorf("%q: no name before a parenthesis is no call", src)
		}
	}
	if _, _, ok := callBefore("<?php\nf(", 7); !ok {
		t.Error("a parenthesis that starts the lexed text is looked for again from the file's start")
	}
}

func TestCallAtReadsTheNamespaceOfTheCall(t *testing.T) {
	indexed := "<?php\nnamespace A;\nuse function B\\f as g;\nclass Room {\n    function run() {\n        g(1, 2);\n    }\n}\n"
	idx := symbols.NewIndex()
	idx.IndexFile("file:///room.php", indexed)
	for _, tc := range []struct{ name, source string }{
		{"the indexed text", indexed},
		{"an edit after the declaration", strings.Replace(indexed, "        g(1", "        $x = 1;\n        g(1", 1)},
		{"an edit before the declaration", strings.Replace(indexed, "Room", "Rooms", 1)},
		{"a file that isn't indexed", indexed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uri := "file:///room.php"
			if tc.name == "a file that isn't indexed" {
				uri = "file:///other.php"
			}
			offset := strings.Index(tc.source, "1, ") + len("1, ")
			pos := protocol.Position{Line: strings.Count(tc.source[:offset], "\n"), Character: offset - strings.LastIndex(tc.source[:offset], "\n") - 1}
			call, ok := CallAt(idx, uri, tc.source, pos)
			if !ok || call.Name != "g" || call.Argument != 1 || call.Callee != CalleeFunction {
				t.Fatalf("got %+v, want g's second argument", call)
			}
			if call.Namespace.Name != "A" || len(call.Namespace.Uses) != 1 || call.Namespace.Uses[0].Alias != "g" {
				t.Errorf("namespace %+v, want A with g imported", call.Namespace)
			}
			if want := (protocol.Position{Line: pos.Line, Character: pos.Character - len("g(1, ")}); call.Position != want {
				t.Errorf("callee at %v, want %v", call.Position, want)
			}
		})
	}
}

func TestCallAtReadsConstructionsAndNamedArguments(t *testing.T) {
	cases := []struct {
		src      string
		ok       bool
		callee   string
		kind     Callee
		argument int
		named    string
	}{
		{"new Room(|", true, "Room", CalleeName, 0, ""},
		{"new \\App\\Room($a, |", true, `\App\Room`, CalleeName, 1, ""},
		{"new static(|", true, "static", CalleeName, 0, ""},
		{"new self(1, |", true, "self", CalleeName, 1, ""},
		{"new parent(|", true, "parent", CalleeName, 0, ""},
		{"new class(1, |", true, "class", CalleeAnonymousClass, 1, ""},
		{"new readonly class(|", true, "class", CalleeAnonymousClass, 0, ""},
		{"#[ORM\\Table(name: 'x', |", true, `ORM\Table`, CalleeName, 1, ""},
		{"#[ORM\\Table(schema: |", true, `ORM\Table`, CalleeName, 0, "schema"},
		{"new Room(size: [1, |", true, "Room", CalleeName, 0, "size"},
		{"area(1, open: f(2), size: |", true, "area", CalleeFunction, 2, "size"},
		{"area(1, /* x */ class : |", true, "area", CalleeFunction, 1, "class"},
		{"area($a ? b : |", true, "area", CalleeFunction, 0, ""},
		{"area(A::B, |", true, "area", CalleeFunction, 1, ""},
		{"area(size|", true, "area", CalleeFunction, 0, ""},
		{"static(|", false, "", CalleeName, 0, ""},
		{"$x = static fn() => static(|", false, "", CalleeName, 0, ""},
		{"class(|", false, "", CalleeName, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			offset := strings.Index(src, "|")
			src = src[:offset] + src[offset+1:]
			pos := protocol.Position{Line: strings.Count(src[:offset], "\n"), Character: offset - strings.LastIndex(src[:offset], "\n") - 1}
			call, ok := CallAt(symbols.NewIndex(), "file:///a.php", src, pos)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, tc.ok, call)
			}
			if !ok {
				return
			}
			if call.Name != tc.callee || call.Callee != tc.kind || call.Argument != tc.argument || call.Named != tc.named {
				t.Errorf("got %q kind %d argument %d named %q; want %q kind %d argument %d named %q",
					call.Name, call.Callee, call.Argument, call.Named, tc.callee, tc.kind, tc.argument, tc.named)
			}
		})
	}
}
