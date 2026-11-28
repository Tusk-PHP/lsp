package sourcectx

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
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

func positionAfter(t *testing.T, source, needle string) protocol.Position {
	t.Helper()
	for i, line := range strings.Split(source, "\n") {
		if col := strings.Index(line, needle); col >= 0 {
			return protocol.Position{Line: i, Character: col + len(needle)}
		}
	}
	t.Fatalf("%q is not in the source", needle)
	return protocol.Position{}
}

func TestAnalyzeBuildsClosureScopeWithUseClause(t *testing.T) {
	source := `<?php
$f = function ($a) use ($b) {
    return $a . $b;
};
$after = 1;
`
	if ctx := Analyze("file:///c.php", source, positionAfter(t, source, "return $")); ctx.Scope == nil || ctx.Scope.Kind != ScopeClosure || ctx.Scope.StartLine != 1 || ctx.Scope.EndLine != 3 {
		t.Errorf("inside the closure body: Scope = %#v, want a closure on lines 1-3", ctx.Scope)
	}
	if ctx := Analyze("file:///c.php", source, positionAfter(t, source, "$aft")); ctx.Scope != nil {
		t.Errorf("after the closure: Scope = %#v, want none", ctx.Scope)
	}
}

func TestAnalyzeArrowFunctionScopeEndsWhereItsExpressionDoes(t *testing.T) {
	for _, tc := range []struct {
		name, source, inside, outside string
	}{
		{"closed by the call's parenthesis", "<?php\nmap(fn($x) => $x + 1); $later;\n", "+ ", "$lat"},
		{"closed by an array's bracket", "<?php\n$fs = [fn($x) => $x * 2]; $later;\n", "* ", "$lat"},
		{"closed by a block's brace", "<?php\nif (true) { $f = fn($x) => $x - 3 } $later;\n", "- ", "$lat"},
		{"running to the end of the file", "<?php\n$f = fn($x) => $x / 4", "/ ", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ctx := Analyze("file:///a.php", tc.source, positionAfter(t, tc.source, tc.inside)); ctx.Scope == nil || ctx.Scope.Kind != ScopeClosure {
				t.Errorf("inside the arrow function: Scope = %#v, want a closure", ctx.Scope)
			}
			if tc.outside == "" {
				return
			}
			if ctx := Analyze("file:///a.php", tc.source, positionAfter(t, tc.source, tc.outside)); ctx.Scope != nil && ctx.Scope.Kind == ScopeClosure {
				t.Errorf("after the arrow function: Scope = %#v, want no closure", ctx.Scope)
			}
		})
	}
}

func TestAnalyzeReadsFunctionConstAndAliasedImports(t *testing.T) {
	source := `<?php
namespace App;
use function App\Support\helper;
use const App\Support\LIMIT;
use App\Models\User as Account;
$x = 1;
`
	ctx := Analyze("file:///u.php", source, positionAfter(t, source, "$x"))
	want := []parser.UseNode{
		{FullName: "App\\Support\\helper", Alias: "helper", Kind: "function", StartLine: 2},
		{FullName: "App\\Support\\LIMIT", Alias: "LIMIT", Kind: "const", StartLine: 3},
		{FullName: "App\\Models\\User", Alias: "Account", Kind: "class", StartLine: 4},
	}
	if !reflect.DeepEqual(ctx.Uses, want) {
		t.Errorf("Uses = %#v, want %#v", ctx.Uses, want)
	}
}

func BenchmarkNewDocument(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nclass Big {\n")
	for range 300 {
		src.WriteString("    public function m(array $a): int { return count($a) + $this->n(); }\n")
	}
	src.WriteString("}\n")
	source := src.String()
	b.ReportAllocs()
	for b.Loop() {
		if NewDocument("file:///a.php", source).File() == nil {
			b.Fatal("the file parses")
		}
	}
}

func TestDocumentOutsideItsLines(t *testing.T) {
	source := "<?php\nnamespace App;\nclass Shop {}\n"
	doc := NewDocument("file:///a.php", source)
	if file := doc.File(); file == nil || len(file.Classes) != 1 || file.Classes[0].Name != "Shop" {
		t.Fatalf("the document's declarations are the parsed file's: %+v", file)
	}
	if doc.URI() != "file:///a.php" {
		t.Errorf("the document's URI is the one it was made with, got %q", doc.URI())
	}
	if name := Analyze("file:///a.php", source, protocol.Position{Line: 2, Character: 7}).Name(); name == nil || name.SymbolText != "Shop" {
		t.Errorf("the name under the cursor is Shop, got %+v", name)
	}
	for _, pos := range []protocol.Position{{Line: -1}, {Line: 10}} {
		if ctx := doc.At(pos); ctx != nil {
			t.Errorf("At(%v) = %+v, want nil", pos, ctx)
		}
		if name := Analyze("file:///a.php", source, pos).Name(); name != nil {
			t.Errorf("Analyze(%v).Name() = %+v, want nil", pos, name)
		}
	}
}
