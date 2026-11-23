package lsp

import "testing"

// memberOwnerCases check that hover, go-to-definition and unknown-member look a
// member up on the same receiver completion sees.
var memberOwnerCases = []phpVersionCase{
	{name: "union/hover on a member of the second class", hover: []string{"name"}, src: `
function run(bool $c): void { if ($c) { $x = new Logger(); } else { $x = new Person(); } $x->na<|>me(); }`},
	{name: "union/definition of a member of the second class", definition: true, src: `
function run(bool $c): void { if ($c) { $x = new Logger(); } else { $x = new Person(); } $x->na<|>me(); }`},
	{name: "called closure/completion after it", completion: []string{"name"}, src: `
function run(): void { $g = fn (int $i): Person => new Person(); $g(1)-><|> }`},
	{name: "called closure/hover on the variable its result is assigned to", hover: []string{"Person"}, src: `
function run(): void { $g = fn (int $i): Person => new Person(); $x = $g(1); echo $<|>x; }`},
	{name: "called closure/hover on the closure variable", hover: []string{"Closure"}, src: `
function run(): void { $g = fn (int $i): Person => new Person(); echo $<|>g; }`},
	{name: "called closure/hover on a member of its result", hover: []string{"name"}, src: `
function run(): void { $g = fn (int $i): Person => new Person(); $g(1)->na<|>me(); }`},
	{name: "called closure/definition of a member of its result", definition: true, src: `
function run(): void { $g = fn (int $i): Person => new Person(); $g(1)->na<|>me(); }`},
	{name: "called closure/an unknown member of its result", diag: []string{"unknown-member"}, src: `
function run(): void { $g = fn (int $i): Person => new Person(); $g(1)->nope(); }`},
}

func TestMemberOwnerAgreesAcrossFeatures(t *testing.T) {
	for _, tc := range memberOwnerCases {
		t.Run(tc.name, func(t *testing.T) {
			runPHPVersionCase(t, tc)
		})
	}
}
