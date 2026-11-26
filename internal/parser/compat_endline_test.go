package parser

import (
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"testing"
)

func TestDeclarationsEndOnTheirClosingLine(t *testing.T) {
	source := `<?php
namespace App;

class Example {
    public function greet(): void {
    }
    abstract public function shape();
}

interface Shape
{
    public function area(): float;
}

trait Named
{
    public function name(): string { return ''; }
}

enum Suit
{
    case Hearts;
}

function helper(): void
{
}
$x = new class {
};
`
	file := ParseFile(source, token.Latest)
	if len(file.Classes) != 1 || len(file.Interfaces) != 1 || len(file.Traits) != 1 || len(file.Enums) != 1 || len(file.Functions) != 1 || len(file.AnonymousClasses) != 1 {
		t.Fatalf("want one of each declaration, got %+v", file)
	}
	checks := []struct {
		name       string
		start, end int
		want       [2]int
	}{
		{"class", file.Classes[0].StartLine, file.Classes[0].EndLine, [2]int{3, 7}},
		{"method", file.Classes[0].Methods[0].StartLine, file.Classes[0].Methods[0].EndLine, [2]int{4, 5}},
		{"abstract method", file.Classes[0].Methods[1].StartLine, file.Classes[0].Methods[1].EndLine, [2]int{6, 6}},
		{"interface", file.Interfaces[0].StartLine, file.Interfaces[0].EndLine, [2]int{9, 12}},
		{"trait", file.Traits[0].StartLine, file.Traits[0].EndLine, [2]int{14, 17}},
		{"enum", file.Enums[0].StartLine, file.Enums[0].EndLine, [2]int{19, 22}},
		{"function", file.Functions[0].StartLine, file.Functions[0].EndLine, [2]int{24, 26}},
		{"anonymous class", file.AnonymousClasses[0].StartLine, file.AnonymousClasses[0].EndLine, [2]int{27, 28}},
	}
	for _, c := range checks {
		if got := [2]int{c.start, c.end}; got != c.want {
			t.Errorf("%s: lines %v, want %v", c.name, got, c.want)
		}
	}
}
