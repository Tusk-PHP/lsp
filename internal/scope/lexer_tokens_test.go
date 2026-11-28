package scope

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestImportsAreNamedByTheirLastSegment(t *testing.T) {
	source := "<?php\nuse App\\Models\\User;\nuse function App\\Support\\helper;\nuse App\\{Http\\Request, Jobs\\Job as Task};\n"
	doc := Collect(source)
	want := map[string]textpos.Range{}
	for _, name := range []string{"User;", "helper;", "Request,", "Task}"} {
		start := at(t, source, name)
		want[name[:len(name)-1]] = textpos.Range{Start: start, End: textpos.Position{Line: start.Line, Character: start.Character + len(name) - 1}}
	}
	got := map[string]textpos.Range{}
	for _, b := range doc.Bindings {
		if b.Kind == BindingImport {
			got[b.Name] = b.Decl
		}
	}
	if len(got) != len(want) {
		t.Errorf("imports = %v, want %v", got, want)
	}
	for name, rng := range want {
		if got[name] != rng {
			t.Errorf("import %s at %v, want %v", name, got[name], rng)
		}
	}
}

func TestUseWithoutAnAliasNameBindsNothing(t *testing.T) {
	for _, b := range Collect("<?php\nuse App\\User as;\n").Bindings {
		if b.Kind == BindingImport {
			t.Errorf("import %q from a use without an alias name", b.Name)
		}
	}
}

func TestSourceWithoutAnOpenTagIsCode(t *testing.T) {
	source := "function f($a) {\n    return $a;\n}\n"
	b := Collect(source).BindingAt(at(t, source, "$a;"))
	if b == nil || b.Kind != BindingParameter || b.Scope.Name != "f" {
		t.Fatalf("$a in a snippet = %+v, want f()'s parameter", b)
	}
}

func TestInterpolatedVariablesAreReadFromTheLexer(t *testing.T) {
	source := "<?php\nfunction run($n, $m) {\n    return \"$n {$m['$n']} {$m->f($n)} ${n}\" . `ls $n`;\n}\n"
	doc := Collect(source)
	n := doc.BindingAt(at(t, source, "$n, $m"))
	if n == nil {
		t.Fatal("no binding for $n")
	}
	want := []Occurrence{
		{Range: textpos.Range{Start: at(t, source, "$n, $m"), End: textpos.Position{Line: 1, Character: 15}}},
		{Range: textpos.Range{Start: textpos.Position{Line: 2, Character: 12}, End: textpos.Position{Line: 2, Character: 14}}},
		{Range: textpos.Range{Start: textpos.Position{Line: 2, Character: 33}, End: textpos.Position{Line: 2, Character: 35}}},
		{Range: textpos.Range{Start: textpos.Position{Line: 2, Character: 40}, End: textpos.Position{Line: 2, Character: 41}}, Braced: true},
		{Range: textpos.Range{Start: at(t, source, "$n`"), End: textpos.Position{Line: 2, Character: 52}}},
	}
	got := doc.Occurrences(n)
	if len(got) != len(want) {
		t.Fatalf("occurrences = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("occurrence %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAMethodNamedCatchDeclaresNoCatchVariable(t *testing.T) {
	source := "<?php\nfunction f($p) {\n    $p->catch(function ($e) { return $e; });\n    return $e;\n}\n"
	doc := Collect(source)
	if b := doc.BindingAt(afterReturn(t, source)); b != nil {
		t.Errorf("$e after ->catch(…) = %+v, want no binding", b)
	}
	if b := doc.BindingAt(at(t, source, "$e) {")); b == nil || b.Kind != BindingParameter || b.Scope.Kind != ScopeClosure {
		t.Errorf("$e in the closure = %+v, want the closure's parameter", b)
	}
}

func TestAMultiLineValueEndsWhereItEnds(t *testing.T) {
	source := "<?php\n$text = <<<EOT\n    one\n    EOT;\n"
	b := Collect(source).BindingAt(at(t, source, "$text"))
	if b == nil || len(b.Assignments) != 1 {
		t.Fatalf("$text = %+v, want one assignment", b)
	}
	want := textpos.Range{Start: at(t, source, "<<<"), End: textpos.Position{Line: 3, Character: 7}}
	if got := b.Assignments[0].Expression; got != want {
		t.Errorf("expression = %v, want %v", got, want)
	}
}

func TestOperatorsReadAsSingleTokens(t *testing.T) {
	source := "<?php\nfunction f($a, $k, $o) {\n    $a ??= 1;\n    $o?->$k = 2;\n    $ok = $k && $b = 3;\n    $or = $k || $c = 4;\n    $nil = $k ?? $d = 5;\n    return [$a, $b, $c, $d];\n}\n"
	doc := Collect(source)
	a := doc.BindingAt(at(t, source, "$a ??="))
	if a == nil || len(a.Rebinds) != 1 || !a.Rebinds[0].Conditional {
		t.Errorf("$a ??= 1: %+v, want one conditional rebind", a)
	}
	if k := doc.BindingAt(at(t, source, "$k = 2")); k == nil || k.Kind != BindingParameter || len(k.Writes) != 0 {
		t.Errorf("$k in $o?->$k = 2: %+v, want the parameter, read rather than written", k)
	}
	for _, name := range []string{"$b = 3", "$c = 4", "$d = 5"} {
		b := doc.BindingAt(at(t, source, name))
		if b == nil || b.Kind != BindingVariable {
			t.Errorf("%s: %+v, want a variable", name, b)
		}
	}
}

func TestACloseTagEndsAStatement(t *testing.T) {
	source := "<?php $b = 0; $a = 1 ?>\n<p><?= $a ?></p>\n"
	b := Collect(source).BindingAt(at(t, source, "$a = 1"))
	if b == nil || len(b.Assignments) != 1 || !b.Assignments[0].DirectStatement {
		t.Fatalf("$a = 1 ?> : %+v, want one direct-statement assignment", b)
	}
	if got := len(b.References); got != 1 {
		t.Errorf("$a has %d references, want 1", got)
	}
}

func TestGotoIsAJumpOutOfTheBlock(t *testing.T) {
	source := "<?php\nfunction f($c) {\n    $x = 1;\n    if ($c) {\n        $x = 2;\n        goto end;\n        $x = 3;\n    }\n    end:\n    return $x;\n}\n"
	doc := Collect(source)
	if len(doc.jumps) != 1 || doc.jumps[0].block != -1 {
		t.Errorf("jumps = %+v, want one goto with an unknown target", doc.jumps)
	}
}

func afterReturn(t *testing.T, source string) textpos.Position {
	t.Helper()
	p := at(t, source, "    return $e;")
	p.Character += len("    return ")
	return p
}
