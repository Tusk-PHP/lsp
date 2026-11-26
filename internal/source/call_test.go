package sourcectx

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
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
			ctx := Analyze("file:///a.php", src, protocol.Position{Line: line, Character: char})
			if ctx == nil {
				t.Fatal("expected cursor context")
			}
			if ctx.FunctionCall != tc.want {
				t.Fatalf("FunctionCall = %v, want %v (word %q)", ctx.FunctionCall, tc.want, ctx.SymbolText)
			}
		})
	}
}

func TestFunctionCallWithoutTokens(t *testing.T) {
	if isCalled(nil, protocol.Range{End: protocol.Position{Line: 1, Character: 4}}) {
		t.Error("no tokens, no call")
	}
}

func TestCallAtFindsTheCalleeAndArgumentFromTokens(t *testing.T) {
	cases := []struct {
		src      string
		ok       bool
		callee   string
		argument int
	}{
		{"area(|", true, "area", 0},
		{"area($a, 'x, y', |", true, "area", 2},
		{"area(f(1, 2), |", true, "area", 1},
		{"area([1, 2, |", true, "area", 0},
		{"area(\n    $a,\n    |", true, "area", 1},
		{"$r->area($a, /* , */ |", true, "area", 1},
		{"area(1); |", false, "", 0},
		{"function () { |", false, "", 0},
		{"area(function () { $x = [1]; }, |", true, "area", 1},
		{"$fn(|", false, "", 0},
		{"|", false, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			offset := strings.Index(src, "|")
			src = src[:offset] + src[offset+1:]
			pos := protocol.Position{Line: strings.Count(src[:offset], "\n"), Character: offset - strings.LastIndex(src[:offset], "\n") - 1}
			callee, argument, ok := CallAt("file:///a.php", src, pos)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if callee.SymbolText != tc.callee || argument != tc.argument {
				t.Errorf("callee %q, argument %d; want %q, %d", callee.SymbolText, argument, tc.callee, tc.argument)
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
		if got := Analyze("file:///a.php", full, pos).Called; got != want {
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
		if got := Analyze("file:///a.php", full, pos).MemberAccess(); got != want {
			t.Errorf("%s: MemberAccess = %v, want %v", src, got, want)
		}
	}
}

func TestCalledVariableAtFindsOnlyAVariableBeingCalled(t *testing.T) {
	cases := []struct {
		src      string
		name     string
		argument int
	}{
		{"$fn(|", "$fn", 0},
		{"$fn(1, |", "$fn", 1},
		{"$fn([1, 2], f(3), |", "$fn", 2},
		{"$x = $greet('Hi', |)", "$greet", 1},
		{"$obj->$method(|", "", 0},
		{"Foo::$method(|", "", 0},
		{"new $class(|", "", 0},
		{"area(|", "", 0},
		{"(|", "", 0},
		{"$fn(1); |", "", 0},
	}
	for _, tc := range cases {
		cursor := strings.Index(tc.src, "|")
		src := "<?php\n" + tc.src[:cursor] + tc.src[cursor+1:]
		doc := NewDocument("file:///t.php", src)
		name, at, argument, ok := doc.CalledVariableAt(protocol.Position{Line: 1, Character: cursor})
		if tc.name == "" {
			if ok {
				t.Errorf("%q: want no called variable, got %s", tc.src, name)
			}
			continue
		}
		if !ok || name != tc.name || argument != tc.argument {
			t.Errorf("%q: got %q argument %d (ok %v), want %q argument %d", tc.src, name, argument, ok, tc.name, tc.argument)
			continue
		}
		if got := doc.Lines()[at.Line][at.Character:]; !strings.HasPrefix(got, tc.name) {
			t.Errorf("%q: position points at %q, want the variable", tc.src, got)
		}
	}
}

func TestCallAtFindsANamedCallee(t *testing.T) {
	src := "<?php\narea(1, f(2), "
	callee, argument, ok := CallAt("file:///t.php", src, protocol.Position{Line: 1, Character: len("area(1, f(2), ")})
	if !ok || callee == nil || argument != 2 {
		t.Fatalf("got callee %v argument %d ok %v, want area's third argument", callee, argument, ok)
	}
	if _, _, ok := CallAt("file:///t.php", "<?php\n$fn(1, ", protocol.Position{Line: 1, Character: len("$fn(1, ")}); ok {
		t.Error("a variable being called has no named callee")
	}
}

func TestDeclaredMarksTheNameAFunctionOrMethodDeclares(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"function ar|ea() {}", true},
		{"function &ar|ea() {}", true},
		{"class R { public function co|unt(): int { return 0; } }", true},
		{"class R { abstract protected static function re|set(); }", true},
		{"ar|ea(1);", false},
		{"$this->ar|ea();", false},
		{"new Ar|ea();", false},
		{"$f = function () use ($ar|ea) {};", false},
		{"/** function ar|ea() */", false},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			offset := strings.Index(src, "|")
			src = src[:offset] + src[offset+1:]
			line := strings.Count(src[:offset], "\n")
			char := offset - strings.LastIndex(src[:offset], "\n") - 1
			ctx := Analyze("file:///a.php", src, protocol.Position{Line: line, Character: char})
			if ctx == nil {
				t.Fatal("expected cursor context")
			}
			if ctx.Declared != tc.want {
				t.Errorf("Declared = %v, want %v", ctx.Declared, tc.want)
			}
		})
	}
}
