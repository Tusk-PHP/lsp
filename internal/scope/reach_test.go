package scope

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// reachingLines returns the lines (1-based) of the writes to name that reach
// the "/*?*/" marker.
func reachingLines(t *testing.T, src, name string) ([]int, Reach) {
	t.Helper()
	lines := strings.Split(src, "\n")
	pos := protocol.Position{Line: -1}
	for i, l := range lines {
		if c := strings.Index(l, "/*?*/"); c >= 0 {
			pos = protocol.Position{Line: i, Character: c + len("/*?*/")}
			break
		}
	}
	if pos.Line < 0 {
		t.Fatal("no /*?*/ marker")
	}
	doc := Collect(src)
	var b *Binding
	for _, cand := range doc.Bindings {
		if cand.Name == name && cand.Scope.Kind != ScopeFile {
			b = cand
		}
	}
	if b == nil {
		t.Fatalf("no binding for %s", name)
	}
	reach := doc.ReachingWrites(b, pos)
	var got []int
	for _, w := range reach.Writes {
		at := b.Decl.Start
		if w != DeclWrite {
			at = b.Rebinds[w].At
		}
		got = append(got, at.Line+1)
	}
	return got, reach
}

func TestReachingWrites(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []int // the lines of the writes that reach the read
	}{
		{"if and else both assign", `<?php
function f($c) {
    if ($c) {
        $x = new Foo();
    } else {
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{4, 6}},
		{"switch", `<?php
function f($c) {
    switch ($c) {
        case 1:
            $x = new Foo();
            break;
        default:
            $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{5, 8}},
		{"an if without else keeps the earlier value", `<?php
function f($c) {
    $x = new Foo();
    if ($c) {
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{3, 5}},
		{"a while loop may not run", `<?php
function f($c) {
    $x = new Foo();
    while ($c) {
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{3, 5}},
		{"inside the branch the write holds", `<?php
function f($c) {
    $x = new Foo();
    if ($c) {
        $x = new Bar();
        /*?*/$x->a();
    }
}`, []int{5}},
		{"every branch assigns: the earlier value is gone", `<?php
function f($c, $d) {
    $x = new Baz();
    if ($c) {
        $x = new Foo();
    } elseif ($d) {
        $x = new Qux();
    } else {
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{5, 7, 9}},
		{"else if written as two words chains too", `<?php
function f($c, $d) {
    $x = new Baz();
    if ($c) {
        $x = new Foo();
    } else if ($d) {
        $x = new Qux();
    } else {
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{5, 7, 9}},
		{"no else: the earlier value still reaches", `<?php
function f($c, $d) {
    $x = new Baz();
    if ($c) {
        $x = new Foo();
    } elseif ($d) {
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{3, 5, 7}},
		{"a branch that doesn't assign keeps the earlier value", `<?php
function f($c) {
    $x = new Baz();
    if ($c) {
        $x = new Foo();
    } else {
        echo 1;
    }
    /*?*/$x->a();
}`, []int{3, 5}},
		{"a later write in the same branch replaces an earlier one", `<?php
function f($c) {
    $x = new Baz();
    if ($c) {
        $x = new Foo();
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{3, 6}},
		{"a break between two writes leaves the first reaching", `<?php
function f($c, $list) {
    $x = new Baz();
    foreach ($list as $item) {
        $x = new Foo();
        if ($c) {
            break;
        }
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{3, 5, 9}},
		{"a break that stays inside the stretch doesn't", `<?php
function f($c, $list) {
    $x = new Baz();
    if ($c) {
        $x = new Foo();
        foreach ($list as $item) {
            break;
        }
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{3, 9}},
		{"an exception in a try block may skip its later writes", `<?php
function f() {
    $x = new Baz();
    try {
        $x = new Foo();
        g();
        $x = new Bar();
    } catch (\Exception $e) {
        /*?*/$x->a();
    }
}`, []int{3, 5, 7}},
		{"a write later in a loop reaches an earlier read in it", `<?php
function f($list) {
    $x = new Foo();
    foreach ($list as $item) {
        /*?*/$x->a();
        $x = new Bar();
    }
}`, []int{3, 6}},
		{"a write earlier in the loop body holds", `<?php
function f($list) {
    $x = new Foo();
    foreach ($list as $item) {
        $x = new Baz();
        /*?*/$x->a();
        $x = new Bar();
    }
}`, []int{5}},
		{"a case label lets code in without the write", `<?php
function f($c) {
    $x = new Baz();
    switch ($c) {
        case 1:
            $x = new Foo();
        case 2:
            /*?*/$x->a();
    }
}`, []int{3, 6}},
		{"a braceless if may not run", `<?php
function f($c) {
    $x = new Foo();
    if ($c) $x = new Bar();
    /*?*/$x->a();
}`, []int{3, 4}},
		{"a parameter keeps its declaration as a value", `<?php
function f(Foo $x, $c) {
    if ($c) {
        $x = new Bar();
    }
    /*?*/$x->a();
}`, []int{2, 4}},
		{"a read in a write's right-hand side sees the value before it", `<?php
function f() {
    $x = new Foo();
    $x = /*?*/$x->next();
}`, []int{3}},
		{"in a loop, it also sees the write itself through the next iteration", `<?php
function f($list) {
    $x = new Foo();
    foreach ($list as $item) {
        $x = /*?*/$x->next();
    }
}`, []int{3, 5}},
		{"a write after && may not run", `<?php
function f($c) {
    $x = new Foo();
    $c && $x = new Bar();
    /*?*/$x->a();
}`, []int{3, 4}},
		{"a write in a ternary may not run", `<?php
function f($c) {
    $x = new Foo();
    $c ? ($x = new Bar()) : null;
    /*?*/$x->a();
}`, []int{3, 4}},
		{"a case label in a switch after the write doesn't let code in around it", `<?php
function f($c) {
    $x = new Baz();
    $x = new Foo();
    switch ($c) {
        case 1:
            /*?*/$x->a();
    }
}`, []int{4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := reachingLines(t, tt.src, "$x")
			if !slices.Equal(got, tt.want) {
				t.Errorf("writes reaching the read are on lines %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReachingWritesOnTheWriteItself(t *testing.T) {
	src := `<?php
function f($c) {
    $x = new Foo();
    if ($c) {
        $x = new Bar();
    }
}`
	doc := Collect(src)
	var b *Binding
	for _, cand := range doc.Bindings {
		if cand.Name == "$x" {
			b = cand
		}
	}
	// On the `x` of the second `$x`.
	reach := doc.ReachingWrites(b, protocol.Position{Line: 4, Character: 9})
	if !reach.OnWrite || !slices.Equal(reach.Writes, []int{0}) {
		t.Errorf("on the write: got %+v, want only that write", reach)
	}
	want := WriteSite{
		At:        protocol.Position{Line: 4, Character: 8},
		RHS:       protocol.Position{Line: 4, Character: 13},
		End:       protocol.Position{Line: 4, Character: 22},
		Statement: true,
	}
	if got := b.Site(0); got != want {
		t.Errorf("Site = %+v, want %+v", got, want)
	}
}

// A write that's a statement of its own mid-line, and one inside an
// expression.
func TestWriteSites(t *testing.T) {
	src := `<?php
function f($c) {
    if ($c) { $x = new Foo(); }
    foo($x = new Bar(), 1);
}`
	doc := Collect(src)
	var b *Binding
	for _, cand := range doc.Bindings {
		if cand.Name == "$x" {
			b = cand
		}
	}
	at := func(line, char int) protocol.Position { return protocol.Position{Line: line, Character: char} }
	if got, want := b.Site(DeclWrite), (WriteSite{At: at(2, 14), RHS: at(2, 19), End: at(2, 28), Statement: true}); got != want {
		t.Errorf("declaration: Site = %+v, want %+v", got, want)
	}
	if got, want := b.Site(0), (WriteSite{At: at(3, 8), RHS: at(3, 13), End: at(3, 22)}); got != want {
		t.Errorf("argument: Site = %+v, want %+v", got, want)
	}
	if site, ok := b.AssignmentAround(at(3, 15)); !ok || site.At != at(3, 8) {
		t.Errorf("AssignmentAround in the argument = %+v, %v", site, ok)
	}
	if _, ok := b.AssignmentAround(at(3, 23)); ok {
		t.Error("AssignmentAround after the argument should find none")
	}
}

// A read more writes may reach than MaxReachingWrites is left unknown.
func TestReachingWritesTooMany(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<?php\nfunction f($n) {\n    $x = new Foo();\n    switch ($n) {\n")
	for i := 0; i <= MaxReachingWrites; i++ {
		sb.WriteString("        case 1:\n            $x = new Bar();\n            break;\n")
	}
	sb.WriteString("    }\n    /*?*/$x->a();\n}")
	got, reach := reachingLines(t, sb.String(), "$x")
	if !reach.TooMany || len(got) != 0 || reach.Single() {
		t.Errorf("got %v, %+v; want TooMany and no writes", got, reach)
	}
}

func TestReachingWritesSingle(t *testing.T) {
	src := `<?php
function f($c) {
    $x = new Foo();
    $x = new Bar();
    /*?*/$x->a();
}`
	_, reach := reachingLines(t, src, "$x")
	if !reach.Single() {
		t.Errorf("a straight-line read should be answered by the last write: %+v", reach)
	}
}

// `$this->$key = …` assigns to a property: $key keeps its value.
func TestDynamicMemberNameIsNotAWrite(t *testing.T) {
	src := `<?php
function f($cache) {
    foreach (['a', 'b'] as $key) {
        $this->$key = $cache[$key];
        /*?*/$key;
    }
}`
	got, _ := reachingLines(t, src, "$key")
	if !slices.Equal(got, []int{3}) {
		t.Errorf("writes reaching the read are on lines %v, want only the foreach on 3", got)
	}
}
