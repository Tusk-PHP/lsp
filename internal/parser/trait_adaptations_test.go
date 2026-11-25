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
			{Trait: "B", Method: "smallTalk", InsteadOf: []string{"A", `\Other\C`}},
			{Trait: "A", Method: "bigTalk", InsteadOf: []string{"B"}},
		},
		Aliases: []TraitAlias{
			{Trait: "B", Method: "bigTalk", Alias: "talk"},
			{Method: "sayHello", Visibility: "protected"},
			{Trait: "A", Method: "list", Alias: "aliased", Visibility: "private"},
			{Trait: "B", Method: "seal", Alias: "sealed", Final: true},
			{Method: "lock", Final: true},
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
