package parser

import (
	"reflect"
	"testing"
)

func TestTraitAdaptationsAreRead(t *testing.T) {
	file := ParseFile(`<?php
class Talker {
    use A, B, C {
        B::smallTalk insteadof A, \Other\C;
        A::bigTalk insteadof B;
        B::bigTalk as talk;
        sayHello as protected;
        /* why */ A::list as private aliased;
        B::seal as final sealed;
        lock as final;
    }
    public function own() {}
}`)
	if len(file.Classes) != 1 {
		t.Fatalf("classes = %d, want 1", len(file.Classes))
	}
	cls := file.Classes[0]
	want := TraitAdaptations{
		Precedences: []TraitPrecedence{
			{Trait: "B", Method: "smallTalk", InsteadOf: []string{"A", `\Other\C`}, MethodLine: 3, MethodCol: 11},
			{Trait: "A", Method: "bigTalk", InsteadOf: []string{"B"}, MethodLine: 4, MethodCol: 11},
		},
		Aliases: []TraitAlias{
			{Trait: "B", Method: "bigTalk", Alias: "talk", MethodLine: 5, MethodCol: 11},
			{Method: "sayHello", Visibility: "protected", MethodLine: 6, MethodCol: 8},
			{Trait: "A", Method: "list", Alias: "aliased", Visibility: "private", MethodLine: 7, MethodCol: 21},
			{Trait: "B", Method: "seal", Alias: "sealed", Final: true, MethodLine: 8, MethodCol: 11},
			{Method: "lock", Final: true, MethodLine: 9, MethodCol: 8},
		},
	}
	if !reflect.DeepEqual(cls.TraitRules, want) {
		t.Errorf("TraitRules = %+v, want %+v", cls.TraitRules, want)
	}
	if !reflect.DeepEqual(cls.Traits, []string{"A", "B", "C"}) {
		t.Errorf("Traits = %v", cls.Traits)
	}
	if len(cls.Methods) != 1 || cls.Methods[0].Name != "own" {
		t.Errorf("Methods = %+v, want only own()", cls.Methods)
	}
}

func TestTraitAdaptationsInTraitsAndEnums(t *testing.T) {
	file := ParseFile(`<?php
trait Both { use A, B { A::run insteadof B; } }
enum Suit { use A { run as go; } case Hearts; }`)
	if len(file.Traits) != 1 || len(file.Traits[0].TraitRules.Precedences) != 1 {
		t.Fatalf("trait rules = %+v", file.Traits)
	}
	if len(file.Enums) != 1 || len(file.Enums[0].TraitRules.Aliases) != 1 || len(file.Enums[0].Cases) != 1 {
		t.Fatalf("enum = %+v", file.Enums)
	}
}

func TestUnterminatedTraitAdaptationBlockKeepsParsing(t *testing.T) {
	file := ParseFile(`<?php
class Broken {
    use A { A::foo insteadof
}
class After {}`)
	var names []string
	for _, c := range file.Classes {
		names = append(names, c.Name)
	}
	if len(names) == 0 || names[0] != "Broken" {
		t.Fatalf("classes = %v", names)
	}
}

func TestTraitRuleMethodAtFindsTheRuleUnderThePosition(t *testing.T) {
	file := ParseFile(`<?php
namespace App;
class Named { use A { talk as chat; } }
trait Mixed { use A, B { B::talk insteadof A; } }
enum Kind { use A { talk as protected; } }
$o = new class { use A { A::talk as private hush; } };
`)
	for _, c := range []struct {
		line, col            int
		owner, trait, method string
	}{
		{2, 22, `App\Named`, "", "talk"},
		{2, 26, `App\Named`, "", "talk"},
		{3, 28, `App\Mixed`, "B", "talk"},
		{4, 20, `App\Kind`, "", "talk"},
		{5, 28, file.AnonymousClasses[0].FullName, "A", "talk"},
	} {
		owner, trait, method, ok := file.TraitRuleMethodAt(c.line, c.col)
		if !ok || owner != c.owner || trait != c.trait || method != c.method {
			t.Errorf("TraitRuleMethodAt(%d, %d) = %q, %q, %q, %v; want %q, %q, %q", c.line, c.col, owner, trait, method, ok, c.owner, c.trait, c.method)
		}
	}
	for _, at := range [][2]int{{2, 30}, {2, 21}, {3, 25}, {1, 0}} {
		if owner, _, _, ok := file.TraitRuleMethodAt(at[0], at[1]); ok {
			t.Errorf("TraitRuleMethodAt(%d, %d) found a rule of %s, want none: not a rule's method name", at[0], at[1], owner)
		}
	}
}
