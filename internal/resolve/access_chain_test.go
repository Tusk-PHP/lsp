package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

// The plain walker gives the class each kind of chain ends with.
func TestExpressionClass(t *testing.T) {
	r, file, pos := chainFixture(t)
	r.Engine = NewChainEngine(r.Index)
	for expr, want := range map[string]string{
		"$p":                       `App\Person`,
		"$p->logger()":             `App\Logger`,
		"$p?->logger()":            `App\Logger`,
		"$this":                    `App\Child`,
		"self::make()":             `App\Child`,
		"static::make()":           `App\Child`,
		"parent::make()":           `App\Child`,
		"Child::make()":            `App\Child`,
		"Base::make()":             `App\Base`,
		"new Person()":             `App\Person`,
		"(new Person())->logger()": `App\Logger`,
		"identity(...)":            "Closure",
		"$p->name":                 "string",
		"$p->missing()":            "",
		"$nothing->logger()":       "",
		"":                         "",
	} {
		if got := r.ExpressionClass(expr, chainSource, pos, file); got != want {
			t.Errorf("ExpressionClass(%q) = %q, want %q", expr, got, want)
		}
	}
}

// Without a file only class names resolve.
func TestExpressionClassWithoutFile(t *testing.T) {
	r, _, pos := chainFixture(t)
	if got := r.ExpressionClass(`\App\Person`, chainSource, pos, nil); got != `App\Person` {
		t.Errorf("a class name: got %q", got)
	}
	if got := r.ExpressionClass("$this", chainSource, pos, nil); got != "" {
		t.Errorf("$this without a file: got %q", got)
	}
}

// A container call is typed by the ContainerCall hook, and one that gives a
// value stops the chain.
func TestAccessChainClassContainerCall(t *testing.T) {
	r, file, pos := chainFixture(t)
	r.ContainerCall = func(_ *Resolver, src string, _ *parser.FileNode, call ast.Expr) (ResolvedType, bool) {
		switch call.Bounds().Text(src) {
		case "app('person')":
			return ResolvedType{FQN: `App\Person`}, true
		case "config('x')":
			return ResolvedType{}, true
		}
		return ResolvedType{}, false
	}
	for expr, want := range map[string]string{
		"app('person')->logger()": `App\Logger`,
		"config('x')->logger()":   "",
	} {
		if got := r.ExpressionClass(expr, chainSource, pos, file); got != want {
			t.Errorf("ExpressionClass(%q) = %q, want %q", expr, got, want)
		}
	}
}

// DiagnosticType gives the full generic-aware type, and the plain class when
// the typed walker finds nothing.
func TestDiagnosticType(t *testing.T) {
	r, file, pos := chainFixture(t)
	e := NewChainEngine(r.Index)
	if got := e.DiagnosticType("$p->logger()", chainSource, pos, file); got != `App\Logger` {
		t.Errorf("a chain: got %q", got)
	}
	if got := e.DiagnosticType("$nothing", chainSource, pos, file); got != "" {
		t.Errorf("an unknown variable: got %q", got)
	}
}

// NewChainEngine resolves chains in assignments with its own walker.
func TestNewChainEngineResolvesAssignedChains(t *testing.T) {
	e := NewChainEngine(nil)
	if e.ChainResolver == nil {
		t.Fatal("NewChainEngine left ChainResolver unset")
	}
}
