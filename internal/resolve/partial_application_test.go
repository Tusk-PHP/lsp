package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func parsedArgs(args string) []CallArg {
	return NewResolver(nil).CallSiteFor(args, ExprContext{}).Args
}

func TestIsPartialApplicationArgs(t *testing.T) {
	for args, want := range map[string]bool{
		"?":               true,
		"'Hello', ?":      true,
		"?, ?, times: 2":  true,
		"'Hi', ...":       true,
		"...":             true, // first-class callable: partial application of nothing
		"attempt: ?":      true,
		"...$args":        false,
		"$a ? $b : $c":    false,
		"$a ?? 'x'":       false,
		"'?'":             false,
		"fn () => $a?->b": false,
		"":                false,
	} {
		if got := isCallableArgs(parsedArgs(args)); got != want {
			t.Errorf("%q: got %v, want %v", args, got, want)
		}
	}
}

func TestPartialClosureKeepsTheOpenParameters(t *testing.T) {
	fn := &symbols.Symbol{Params: []symbols.ParamInfo{{Name: "$greeting"}, {Name: "$name"}, {Name: "$times"}}}
	full := ResolvedType{FQN: "Closure", Callable: &CallableType{
		Params: []ResolvedType{{FQN: "string"}, {FQN: "int"}, {FQN: "float"}},
		Return: &ResolvedType{FQN: "bool"},
	}}
	for args, want := range map[string]string{
		"'Hi', ?":        "Closure(int): bool",
		"?, ?, times: 2": "Closure(string, int): bool",
		"'Hi', ...":      "Closure(int, float): bool",
		"times: ?, ...":  "Closure(string, int, float): bool",
		"...":            "Closure(string, int, float): bool",
		"?, 'x', ?":      "Closure(string, float): bool",
	} {
		if got := PartialClosure(full, fn, parsedArgs(args)).FullString(); got != want {
			t.Errorf("%q: got %s, want %s", args, got, want)
		}
	}
}
