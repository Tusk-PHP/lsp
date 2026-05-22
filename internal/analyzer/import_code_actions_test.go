package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
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
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{diag}, Only: []string{"quickfix"}},
	})

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

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []string{"source.organizeImports"}},
	})

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

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []string{"source"}},
	})

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

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []string{"source.organizeImports"}},
	})

	if len(actions) != 0 {
		t.Fatalf("expected no organize imports action for commented block, got %#v", actions)
	}
}
