package scope

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func at(t *testing.T, source, needle string) textpos.Position {
	t.Helper()
	for i, line := range strings.Split(source, "\n") {
		if col := strings.Index(line, needle); col >= 0 {
			return textpos.Position{Line: i, Character: col}
		}
	}
	t.Fatalf("%q is not in the source", needle)
	return textpos.Position{}
}

func TestFunctionWithReturnTypeOpensItsBody(t *testing.T) {
	source := `<?php
function total(int $a): int /* sum */ {
    return $a;
}
`
	doc := Collect(source)
	s := doc.ScopeAt(at(t, source, "return"))
	if s == nil || s.Kind != ScopeFunction || s.Name != "total" {
		t.Fatalf("scope of the body = %+v, want function total", s)
	}
	a := doc.BindingAt(at(t, source, "$a)"))
	if a == nil || a.Kind != BindingParameter || len(doc.Occurrences(a)) != 2 {
		t.Errorf("parameter $a = %+v, want a parameter read once in the body", a)
	}
}

func TestCatchTypesSkipComments(t *testing.T) {
	source := `<?php
function run() {
    try {} catch (First $e) {}
    try {} catch (Second /* why */ | \Third\Error $e) { return $e; }
}
`
	doc := Collect(source)
	e := doc.BindingAt(at(t, source, "$e)"))
	if e == nil || len(e.Rebinds) != 1 {
		t.Fatalf("$e = %+v, want one rebind by the second catch", e)
	}
	if got, want := e.Rebinds[0].Caught, []string{"Second", "\\Third\\Error"}; !slices.Equal(got, want) {
		t.Errorf("second catch catches %v, want %v", got, want)
	}
}

func TestStaticPropertyAfterNewWithSubscriptIsNotAVariable(t *testing.T) {
	source := `<?php
class C {
    function make($k) {
        return new $this->map[$k]::$cls();
    }
}
`
	doc := Collect(source)
	cls := at(t, source, "$cls")
	if !slices.ContainsFunc(doc.Properties, func(r textpos.Range) bool { return r.Start == cls }) {
		t.Errorf("$cls after new $this->map[$k]:: is not a property: %+v", doc.Properties)
	}
	if b := doc.BindingAt(cls); b != nil {
		t.Errorf("$cls is bound as a variable: %+v", b)
	}
}

func TestGlobalAndStaticDeclareEveryListedVariable(t *testing.T) {
	source := `<?php
function run() {
    global $first, $second;
    static $third;
    return $first . $second . $third;
}
`
	doc := Collect(source)
	for _, name := range []string{"$first", "$second", "$third"} {
		b := doc.BindingAt(at(t, source, name))
		if b == nil || b.Scope.Name != "run" {
			t.Errorf("%s: binding %+v, want one in run()", name, b)
			continue
		}
		if got := len(doc.Occurrences(b)); got != 2 {
			t.Errorf("%s: %d occurrences, want the declaration and the read", name, got)
		}
	}
}

func TestPropertyHooksHaveTheirOwnScopes(t *testing.T) {
	source := `<?php
class Person {
    public string $name {
        set(string $raw) { $this->name = trim($raw); }
        get => strtoupper($this->name);
    }
    public string $nick {
        set { $this->nick = $value; }
    }
}
`
	doc := Collect(source)

	raw := doc.BindingAt(at(t, source, "$raw)"))
	if raw == nil || raw.Kind != BindingParameter || raw.Scope.Kind != ScopeMethod || raw.Scope.Name != "set" {
		t.Fatalf("$raw = %+v, want a parameter of the set hook", raw)
	}
	if got := len(doc.Occurrences(raw)); got != 2 {
		t.Errorf("$raw: %d occurrences, want 2", got)
	}

	get := doc.ScopeAt(at(t, source, "strtoupper"))
	if get == nil || get.Kind != ScopeMethod || get.Name != "get" || !get.ExprBody {
		t.Fatalf("scope of the get hook = %+v, want an expression-bodied get", get)
	}
	wantEnd := at(t, source, "strtoupper")
	wantEnd.Character += len("strtoupper($this->name)")
	if get.Range.End != wantEnd {
		t.Errorf("get hook ends at %+v, want %+v", get.Range.End, wantEnd)
	}

	nick := doc.ScopeAt(at(t, source, "$value"))
	if nick == nil || nick.Name != "set" {
		t.Fatalf("scope at $value = %+v, want the second set hook", nick)
	}
	value := doc.BindingAt(at(t, source, "$value"))
	if value == nil || value.Kind != BindingParameter || value.Scope != nick {
		t.Errorf("$value = %+v, want the set hook's implicit parameter", value)
	}
}

func TestNestedSubscriptWriteDeclaresTheVariable(t *testing.T) {
	source := `<?php
function run() {
    $grid[1][2] = 3;
    return $grid;
}
`
	doc := Collect(source)
	b := doc.BindingAt(at(t, source, "$grid["))
	if b == nil || b.Kind != BindingImplicit {
		t.Fatalf("$grid = %+v, want an implicit binding", b)
	}
	if got := len(doc.Occurrences(b)); got != 2 {
		t.Errorf("$grid: %d occurrences, want 2", got)
	}
}

func TestOutArgumentsAskTheCallee(t *testing.T) {
	type call struct {
		callee string
		arg    int
	}
	for _, tc := range []struct {
		name, code string
		want       call
		bound      bool
	}{
		{"a function", `preg_match('/x/', $s, $out);`, call{"preg_match", 2}, true},
		{"a qualified function", `\Vendor\Pkg\fill($out);`, call{"\\Vendor\\Pkg\\fill", 0}, true},
		{"a method", `$this->fill(1, $out);`, call{"", 1}, true},
		{"a static method", `Filler::fill($out);`, call{"", 0}, true},
		{"a constructor", `new Filler($out);`, call{"", 0}, true},
		{"new static", `new static($out);`, call{"", 0}, true},
		{"a callable", `$fill($out);`, call{"", 0}, true},
		{"an argument after a nested call", `fill(f(1, 2), [3], $out);`, call{"fill", 2}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "<?php\nclass Filler { function run() {\n    " + tc.code + "\n    return $out;\n} }\n"
			var asked []call
			c := &Collector{OutArg: func(callee string, arg int) bool {
				asked = append(asked, call{callee, arg})
				return true
			}}
			doc := c.Collect(source)
			if !slices.Contains(asked, tc.want) {
				t.Errorf("OutArg asked %v, want %v among them", asked, tc.want)
			}
			b := doc.BindingAt(at(t, source, "$out"))
			if (b != nil && b.Kind == BindingImplicit) != tc.bound {
				t.Errorf("$out = %+v, want an implicit binding: %v", b, tc.bound)
			}
		})
	}

	for _, code := range []string{
		`if ($out) {}`,
		`fill($out . 'x');`,
	} {
		t.Run("not an out argument: "+code, func(t *testing.T) {
			source := "<?php\nclass Filler { function run() {\n    " + code + "\n} }\n"
			var asked []string
			c := &Collector{OutArg: func(callee string, arg int) bool {
				asked = append(asked, callee)
				return true
			}}
			c.Collect(source)
			if len(asked) != 0 {
				t.Errorf("OutArg was asked for %v", asked)
			}
		})
	}
}

func TestTopLevelAssignmentIsRecorded(t *testing.T) {
	source := "<?php $total = 40 + 2;"
	doc := Collect(source)
	b := doc.BindingAt(at(t, source, "$total"))
	if b == nil || len(b.Assignments) != 1 {
		t.Fatalf("$total = %+v, want one assignment", b)
	}
	want := textpos.Range{Start: at(t, source, "40"), End: textpos.Position{Line: 0, Character: strings.Index(source, ";")}}
	if got := b.Assignments[0].Expression; !reflect.DeepEqual(got, want) {
		t.Errorf("assigned expression = %+v, want %+v", got, want)
	}
}

func TestArrowFunctionRunsToTheEndOfTheFile(t *testing.T) {
	source := "<?php\n$double = fn($x) => $x * 2"
	doc := Collect(source)
	s := doc.ScopeAt(at(t, source, "* 2"))
	if s == nil || s.Kind != ScopeClosure || !s.ExprBody {
		t.Fatalf("scope of the body = %+v, want an arrow function", s)
	}
	if want := (textpos.Position{Line: 1, Character: len("$double = fn($x) => $x * 2")}); s.Range.End != want {
		t.Errorf("arrow function ends at %+v, want %+v", s.Range.End, want)
	}
}
