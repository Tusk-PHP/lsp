package parser

import "testing"

func TestParseAnonymousClasses(t *testing.T) {
	file := ParseFile(`<?php
namespace App\Casts;

class AsCollection implements Castable
{
    public static function castUsing(array $arguments)
    {
        return new class($arguments) extends Base implements CastsAttributes, Other
        {
            public function __construct(protected array $arguments) {}

            public function get($model, $key)
            {
                return new readonly class {
                    public int $depth;
                };
            }
        };
    }
}
`)
	if len(file.Classes) != 1 || file.Classes[0].Name != "AsCollection" {
		t.Fatalf("declared classes = %+v", file.Classes)
	}
	if got := len(file.Classes[0].Methods); got != 1 {
		t.Errorf("AsCollection has %d methods, want 1: the anonymous class's methods aren't its own", got)
	}
	if len(file.AnonymousClasses) != 2 {
		t.Fatalf("got %d anonymous classes, want 2", len(file.AnonymousClasses))
	}
	outer, inner := file.AnonymousClasses[0], file.AnonymousClasses[1]
	if outer.Name != AnonymousClassName || !IsAnonymousClass(outer.FullName) {
		t.Errorf("name %q, full name %q", outer.Name, outer.FullName)
	}
	if outer.Extends != "Base" || len(outer.Implements) != 2 || outer.Implements[0] != "CastsAttributes" {
		t.Errorf("extends %q implements %v", outer.Extends, outer.Implements)
	}
	if outer.StartLine != 7 || outer.EndLine != 17 {
		t.Errorf("outer lines %d-%d, want 7-17", outer.StartLine, outer.EndLine)
	}
	var names []string
	for _, m := range outer.Methods {
		names = append(names, m.Name)
	}
	if len(names) != 2 || names[0] != "__construct" || names[1] != "get" {
		t.Errorf("outer methods %v", names)
	}
	if len(outer.Properties) != 1 || outer.Properties[0].Name != "$arguments" {
		t.Errorf("promoted property: %+v", outer.Properties)
	}
	if !inner.IsReadonly || len(inner.Properties) != 1 || inner.StartLine != 13 || inner.EndLine != 15 {
		t.Errorf("inner: readonly=%v props=%d lines %d-%d", inner.IsReadonly, len(inner.Properties), inner.StartLine, inner.EndLine)
	}
	if outer.FullName == inner.FullName {
		t.Errorf("both named %q", outer.FullName)
	}
}
