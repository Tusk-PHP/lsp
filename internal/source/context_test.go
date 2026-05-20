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
