package analyzer

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func unusedImportCodeActionDiagnostic(line, end int, message string) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: line, Character: 0},
			End:   protocol.Position{Line: line, Character: end},
		},
		Severity: protocol.DiagnosticSeverityHint,
		Source:   "tusk-php",
		Code:     "unused-import",
		Message:  message,
	}
}

func TestGetCodeActionsUnusedImportQuickFixImportSlice(t *testing.T) {
	source := `<?php
use App\Models\User;
use App\Models\Post;

$user = new User();
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	diag := unusedImportCodeActionDiagnostic(2, len("use App\\Models\\Post;"), "Unused import 'App\\Models\\Post'")
	actions := withoutIgnoreActions(a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{diag}, Only: []protocol.CodeActionKind{"quickfix"}},
	}))

	if len(actions) != 1 {
		t.Fatalf("expected one quickfix action, got %#v", actions)
	}
	action := actions[0]
	if action.Title != "Remove unused import" {
		t.Fatalf("unexpected title %q", action.Title)
	}
	if action.Kind != "quickfix" || !action.IsPreferred {
		t.Fatalf("unexpected quickfix metadata: %#v", action)
	}
	edits := action.Edit.Changes["file:///test.php"]
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", action.Edit)
	}
	if edits[0].Range.Start.Line != 2 || edits[0].Range.End.Line != 3 {
		t.Fatalf("unexpected edit range %#v", edits[0].Range)
	}
	if edits[0].NewText != "" {
		t.Fatalf("expected deletion edit, got %q", edits[0].NewText)
	}
}

func TestGetCodeActionsOrganizeImportsImportSlice(t *testing.T) {
	source := `<?php
use const App\Config\MAX_RETRIES;
use App\Models\Post;
use function App\Helpers\formatName;
use App\Models\User;

$user = new User();
$name = formatName('Ada');
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := withoutIgnoreActions(a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"source.organizeImports"}},
	}))

	if len(actions) != 1 {
		t.Fatalf("expected one organize imports action, got %#v", actions)
	}
	action := actions[0]
	if action.Title != "Organize Imports" || action.Kind != "source.organizeImports" {
		t.Fatalf("unexpected organize imports action: %#v", action)
	}
	edits := action.Edit.Changes["file:///test.php"]
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", action.Edit)
	}
	if edits[0].Range.Start.Line != 1 || edits[0].Range.End.Line != 5 {
		t.Fatalf("unexpected organize range %#v", edits[0].Range)
	}
	want := "use App\\Models\\User;\n\nuse function App\\Helpers\\formatName;"
	if edits[0].NewText != want {
		t.Fatalf("unexpected organize replacement %q", edits[0].NewText)
	}
}

func TestGetCodeActionsOrganizeImportsAllowedBySourceParent(t *testing.T) {
	source := `<?php
use App\Models\Post;
use App\Models\User;

$user = new User();
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := withoutIgnoreActions(a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"source"}},
	}))

	for _, action := range actions {
		if action.Kind == "source.organizeImports" {
			return
		}
	}

	t.Fatalf("expected source.organizeImports action when only=source, got %#v", actions)
}

func TestGetCodeActionsOrganizeImportsSkipsUnsafeBlockImportSlice(t *testing.T) {
	source := `<?php
use App\Models\User;
// keep this comment anchored
use App\Models\Post;

$user = new User();
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := withoutIgnoreActions(a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"source.organizeImports"}},
	}))

	if len(actions) != 0 {
		t.Fatalf("expected no organize imports action for commented block, got %#v", actions)
	}
}

func TestGetCodeActionsOrganizeImportsDropsPrefixOfNonASCIIName(t *testing.T) {
	source := "<?php\nuse App\\Caf;\nuse App\\Café;\n\n$x = new Café();\n"
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := withoutIgnoreActions(a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"source.organizeImports"}},
	}))

	if len(actions) != 1 {
		t.Fatalf("expected one organize imports action, got %#v", actions)
	}
	edits := actions[0].Edit.Changes["file:///test.php"]
	if len(edits) != 1 || edits[0].NewText != "use App\\Café;" {
		t.Fatalf("expected only App\\Café to stay, got %#v", edits)
	}
}

func organizeImportsReplacement(t *testing.T, source string) (string, bool) {
	t.Helper()
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})
	actions := withoutIgnoreActions(a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{protocol.CodeActionSourceOrganizeImports}},
	}))
	if len(actions) == 0 {
		return "", false
	}
	if len(actions) != 1 {
		t.Fatalf("expected at most one organize imports action, got %#v", actions)
	}
	edits := actions[0].Edit.Changes["file:///test.php"]
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", actions[0].Edit)
	}
	return edits[0].NewText, true
}

func TestGetCodeActionsOrganizeImportsIgnoresMentionsOutsideCode(t *testing.T) {
	cases := map[string]string{
		"line comment":  "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n// Post is loaded lazily\nnew User();\n",
		"block comment": "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n/* Post */\nnew User();\n",
		"string":        "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\necho 'Post', \"Post\";\nnew User();\n",
		"heredoc":       "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\necho <<<TXT\nPost\nTXT;\nnew User();\n",
		"inline HTML":   "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n?>\n<h1>Post</h1>\n<?php new User();\n",
		"variable":      "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n$Post = new User();\n",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := organizeImportsReplacement(t, source)
			if !ok || got != "use App\\Models\\User;" {
				t.Fatalf("expected only App\\Models\\User to stay, got %q (action %v)", got, ok)
			}
		})
	}
}

func TestGetCodeActionsOrganizeImportsKeepsCodeAndDocReferences(t *testing.T) {
	cases := map[string]string{
		"param tag":     "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n/**\n * @param Post $p\n */\nfunction f($p) { new User(); }\n",
		"var tag":       "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n/** @var Post[] $posts */\n$posts = [new User()];\n",
		"inline var":    "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n/* @var $p Post */\n$p = new User();\n",
		"interpolation": "<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\necho \"{$app->make(Post::class)}\";\nnew User();\n",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if got, ok := organizeImportsReplacement(t, source); ok {
				t.Fatalf("expected both imports to stay, got %q", got)
			}
		})
	}
}

func TestOrganizeImportsAgreesWithUnusedImport(t *testing.T) {
	sources := []string{
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n// Post\nnew User();\n",
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\necho 'Post';\n",
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n?>\nPost User\n",
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n/** @return Post */\nfunction f() { return User::first(); }\n",
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n/* @var $p Post */\n$p = User::first();\n",
		"<?php\nuse App\\Backend\\Action;\nuse App\\Backend\\Action\\Context;\n\nnew Context();\n",
		"<?php\nuse App\\Models\\Post as Article;\nuse App\\Models\\User;\n\nnew Post();\nnew User();\n",
		"<?php\nuse App\\Models;\nuse App\\Models\\User;\n\nnew Models\\Post();\n",
		"<?php\nuse App\\Concerns\\HasPosts;\nuse App\\Models\\User;\n\nclass A { use HasPosts; }\n",
		"<?php\nuse const App\\LIMIT;\nuse function App\\format;\n\necho format('LIMIT');\n",
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\necho \"{$x->find(User::class)}\";\n",
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\nnew \\App\\Models\\Post();\nnew User();\n",
		"<?php\nuse App\\Models;\nuse App\\Models\\User;\n\necho $x->User, $x?->User();\nnew Models\\User();\n",
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\nPost::User();\n",
		"<?php\nuse const App\\LIMIT;\nuse function App\\format;\n\nLIMIT();\necho format;\n",
		"<?php\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\nnew post();\n/** @var user $u */\n",
		"<?php\nuse const App\\LIMIT;\nuse function App\\format;\n\necho FORMAT(limit);\n",
		"<?php\nuse App\\Backend\\Action;\nuse App\\Models\\Post;\nuse App\\Models\\User;\n\n/**\n * Runs the action for a {@see User}.\n * @param array{post: Post} $row\n */\nfunction f($row) {}\n",
	}
	rule := &checks.UnusedImportsRule{}
	for _, source := range sources {
		file := parser.ParseFile(source)
		var flagged []string
		for _, f := range rule.Check(file, source, nil) {
			flagged = append(flagged, strings.TrimSpace(strings.Split(source, "\n")[f.StartLine]))
		}
		var removed []string
		if replacement, ok := organizeImportsReplacement(t, source); ok {
			kept := strings.Split(replacement, "\n")
			for _, u := range file.Uses {
				line := strings.TrimSpace(strings.Split(source, "\n")[u.StartLine])
				if !slices.Contains(kept, line) {
					removed = append(removed, line)
				}
			}
		}
		if !slices.Equal(flagged, removed) {
			t.Errorf("source %q: unused-import flags %q, Organize Imports removes %q", source, flagged, removed)
		}
	}
}

func organizeImportsBenchmarkSource() string {
	var b strings.Builder
	b.WriteString("<?php\nnamespace App\\Http;\n\n")
	for i := range 30 {
		fmt.Fprintf(&b, "use App\\Models\\Model%d;\n", i)
	}
	b.WriteString("\nfinal class Controller\n{\n")
	for i := range 500 {
		fmt.Fprintf(&b, "    /**\n     * @param Model%d $model\n     */\n", i%15)
		fmt.Fprintf(&b, "    public function action%d($model): string\n    {\n", i)
		fmt.Fprintf(&b, "        // Model%d is loaded lazily\n", 15+i%15)
		fmt.Fprintf(&b, "        $label = \"model {$model->name} of Model%d\";\n", 15+i%15)
		b.WriteString("        return $label . 'done';\n    }\n\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func BenchmarkOrganizeImports(b *testing.B) {
	source := organizeImportsBenchmarkSource()
	a, _ := setupRenameAnalyzer(map[string]string{"file:///bench.php": source})
	params := protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///bench.php"},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{protocol.CodeActionSourceOrganizeImports}},
	}
	b.ReportAllocs()
	for b.Loop() {
		a.GetCodeActions("file:///bench.php", source, params)
	}
}
