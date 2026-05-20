package sourcectx

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestAnalyzeBuildsSharedCursorContext(t *testing.T) {
	source := `<?php
namespace App\Services;

use App\Models\User;

class Handler
{
    public function run(User $user): void
    {
        $user
            ->profile()
            ->name();
    }
}`

	ctx := Analyze("file:///handler.php", source, protocol.Position{Line: 11, Character: 14})
	if ctx == nil {
		t.Fatal("expected cursor context")
	}
	if ctx.Namespace != "App\\Services" {
		t.Fatalf("Namespace = %q, want %q", ctx.Namespace, "App\\Services")
	}
	if ctx.SymbolText != "name" {
		t.Fatalf("SymbolText = %q, want %q", ctx.SymbolText, "name")
	}
	if ctx.SymbolKind != ContextSymbolIdentifier {
		t.Fatalf("SymbolKind = %q, want %q", ctx.SymbolKind, ContextSymbolIdentifier)
	}
	if ctx.AccessKind != AccessInstance {
		t.Fatalf("AccessKind = %q, want %q", ctx.AccessKind, AccessInstance)
	}
	if ctx.SubjectExpr != "$user            ->profile()" {
		t.Fatalf("SubjectExpr = %q", ctx.SubjectExpr)
	}
	if ctx.EnclosingFQN != "App\\Services\\Handler" {
		t.Fatalf("EnclosingFQN = %q", ctx.EnclosingFQN)
	}
	if len(ctx.Uses) != 1 || ctx.Uses[0].Alias != "User" {
		t.Fatalf("Uses = %#v, want aliased User import", ctx.Uses)
	}
	if ctx.Scope == nil || ctx.Scope.MethodName != "run" {
		t.Fatalf("Scope = %#v, want run method scope", ctx.Scope)
	}
	if ctx.Scope.Kind != ScopeMethod || ctx.Scope.Name != "run" {
		t.Fatalf("Scope = %#v, want method kind/name", ctx.Scope)
	}
}

func TestWordAtHandlesVariableCursorOnDollar(t *testing.T) {
	source := "<?php\n$variable = 1;\n"
	word, rng := WordAt(source, protocol.Position{Line: 1, Character: 0})
	if word != "$variable" {
		t.Fatalf("WordAt() = %q, want %q", word, "$variable")
	}
	if rng.Start.Character != 0 || rng.End.Character != 9 {
		t.Fatalf("range = %#v", rng)
	}
}

func TestDetectMemberContext(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		prefix string
		filter string
		kind   AccessKind
	}{
		{name: "instance", input: "$user->pro", prefix: "$user->", filter: "pro", kind: AccessInstance},
		{name: "nullsafe", input: "$user?->pro", prefix: "$user?->", filter: "pro", kind: AccessNullsafe},
		{name: "static", input: "User::cre", prefix: "User::", filter: "cre", kind: AccessStatic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DetectMemberContext(tt.input)
			if !ok {
				t.Fatal("expected member context")
			}
			if got.Prefix != tt.prefix || got.Filter != tt.filter || got.AccessKind != tt.kind {
				t.Fatalf("DetectMemberContext() = %#v", got)
			}
		})
	}
}

func TestNamespaceUsesParserResult(t *testing.T) {
	source := "<?php\nnamespace App\\Models;\nclass User {}\n"
	if got := Namespace(source); got != "App\\Models" {
		t.Fatalf("Namespace() = %q, want %q", got, "App\\Models")
	}
}

func TestAnalyzeBuildsFunctionScopeOutsideClass(t *testing.T) {
	source := `<?php
namespace App\Helpers;

use App\Models\User;

function formatUser(User $user): string
{
    return $user->name();
}
`

	ctx := Analyze("file:///helpers.php", source, protocol.Position{Line: 7, Character: 18})
	if ctx == nil {
		t.Fatal("expected cursor context")
	}
	if ctx.Namespace != "App\\Helpers" {
		t.Fatalf("Namespace = %q, want %q", ctx.Namespace, "App\\Helpers")
	}
	if ctx.Scope == nil {
		t.Fatal("expected function scope")
	}
	if ctx.Scope.Kind != ScopeFunction || ctx.Scope.Name != "formatUser" {
		t.Fatalf("Scope = %#v, want formatUser function scope", ctx.Scope)
	}
	if ctx.Scope.ClassFQN != "" || ctx.Scope.MethodName != "" {
		t.Fatalf("Scope = %#v, expected no enclosing class or method", ctx.Scope)
	}
}

func TestAnalyzeBuildsClosureScopeInsideMethod(t *testing.T) {
	source := `<?php
namespace App\Services;

class Handler
{
    public function run(): void
    {
        $fn = function (string $name): string {
            return strtoupper($name);
        };
    }
}
`

	ctx := Analyze("file:///handler.php", source, protocol.Position{Line: 8, Character: 31})
	if ctx == nil {
		t.Fatal("expected cursor context")
	}
	if ctx.EnclosingFQN != "App\\Services\\Handler" {
		t.Fatalf("EnclosingFQN = %q", ctx.EnclosingFQN)
	}
	if ctx.Scope == nil {
		t.Fatal("expected closure scope")
	}
	if ctx.Scope.Kind != ScopeClosure {
		t.Fatalf("Scope.Kind = %q, want %q", ctx.Scope.Kind, ScopeClosure)
	}
	if ctx.Scope.ClassFQN != "App\\Services\\Handler" {
		t.Fatalf("Scope.ClassFQN = %q", ctx.Scope.ClassFQN)
	}
}

func TestAnalyzeBuildsArrowFunctionScopeInsideMethod(t *testing.T) {
	source := `<?php
namespace App\Services;

class Handler
{
    public function run(): void
    {
        $prefix = "user";
        $fn = fn (string $name): string => $prefix . $name;
    }
}
`

	ctx := Analyze("file:///handler.php", source, protocol.Position{Line: 8, Character: 49})
	if ctx == nil {
		t.Fatal("expected cursor context")
	}
	if ctx.EnclosingFQN != "App\\Services\\Handler" {
		t.Fatalf("EnclosingFQN = %q", ctx.EnclosingFQN)
	}
	if ctx.Scope == nil {
		t.Fatal("expected arrow-function scope")
	}
	if ctx.Scope.Kind != ScopeClosure {
		t.Fatalf("Scope.Kind = %q, want %q", ctx.Scope.Kind, ScopeClosure)
	}
	if ctx.Scope.ClassFQN != "App\\Services\\Handler" {
		t.Fatalf("Scope.ClassFQN = %q", ctx.Scope.ClassFQN)
	}
}

func TestAnalyzeUsesPositionAwareNamespaceAndImports(t *testing.T) {
	source := `<?php
namespace App\One;

use App\Shared\Thing;

function first(Thing $thing): void {}

namespace App\Two {
    use App\Other\Tool;

    class Handler
    {
        public function run(Tool $tool): void
        {
            $tool->handle();
        }
    }
}
`

	firstCtx := Analyze("file:///multi.php", source, protocol.Position{Line: 5, Character: 21})
	if firstCtx == nil {
		t.Fatal("expected first cursor context")
	}
	if firstCtx.Namespace != "App\\One" {
		t.Fatalf("first namespace = %q, want %q", firstCtx.Namespace, "App\\One")
	}
	if len(firstCtx.Uses) != 1 || firstCtx.Uses[0].Alias != "Thing" {
		t.Fatalf("first uses = %#v, want Thing import", firstCtx.Uses)
	}

	secondCtx := Analyze("file:///multi.php", source, protocol.Position{Line: 13, Character: 20})
	if secondCtx == nil {
		t.Fatal("expected second cursor context")
	}
	if secondCtx.Namespace != "App\\Two" {
		t.Fatalf("second namespace = %q, want %q", secondCtx.Namespace, "App\\Two")
	}
	if len(secondCtx.Uses) != 1 || secondCtx.Uses[0].Alias != "Tool" {
		t.Fatalf("second uses = %#v, want Tool import", secondCtx.Uses)
	}
	if secondCtx.EnclosingFQN != "App\\Two\\Handler" {
		t.Fatalf("EnclosingFQN = %q, want %q", secondCtx.EnclosingFQN, "App\\Two\\Handler")
	}
}

func TestAnalyzeDoesNotExposeLaterImportsBeforeDeclaration(t *testing.T) {
	source := `<?php
namespace App\One;

class BeforeImports
{
    public function run(): void {}
}

use App\Later\Thing;
`

	ctx := Analyze("file:///imports.php", source, protocol.Position{Line: 4, Character: 20})
	if ctx == nil {
		t.Fatal("expected cursor context")
	}
	if len(ctx.Uses) != 0 {
		t.Fatalf("Uses = %#v, want no active imports before declaration", ctx.Uses)
	}
}
