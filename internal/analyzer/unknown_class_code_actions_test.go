package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func unknownClassDiagnostic(line, start, end int, message string) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: line, Character: start},
			End:   protocol.Position{Line: line, Character: end},
		},
		Severity: protocol.DiagnosticSeverityWarning,
		Source:   "tusk-php",
		Code:     "unknown-class",
		Message:  message,
	}
}

func TestGetCodeActionsUnknownClassImportQuickFix(t *testing.T) {
	source := `<?php
namespace App;

class Demo {
    public function run(): void {
        new Logger();
    }
}
`

	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///test.php": source,
		"file:///vendor/logger.php": `<?php
namespace Monolog;
class Logger {}
`,
	})

	diag := unknownClassDiagnostic(5, 12, 18, "Unknown class 'Logger'")
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{diag}},
	})

	if len(actions) != 2 {
		t.Fatalf("expected quickfix + copy namespace actions, got %d: %#v", len(actions), actions)
	}

	var fix *protocol.CodeAction
	for i := range actions {
		if actions[i].Kind == "quickfix" {
			fix = &actions[i]
			break
		}
	}
	if fix == nil {
		t.Fatal("expected unknown-class quickfix action")
	}
	if fix.Title != "Import Monolog\\Logger" {
		t.Fatalf("unexpected quickfix title %q", fix.Title)
	}
	if !fix.IsPreferred {
		t.Fatal("expected single-candidate quickfix to be preferred")
	}
	if fix.Edit == nil || len(fix.Edit.Changes["file:///test.php"]) != 1 {
		t.Fatalf("expected one edit for import insertion, got %#v", fix.Edit)
	}

	edit := fix.Edit.Changes["file:///test.php"][0]
	if edit.NewText != "use Monolog\\Logger;\n" {
		t.Fatalf("unexpected import edit %q", edit.NewText)
	}
	if edit.Range.Start.Line != 2 || edit.Range.Start.Character != 0 {
		t.Fatalf("unexpected import position %+v", edit.Range.Start)
	}
}

func TestGetCodeActionsUnknownClassImportQuickFixMultipleCandidates(t *testing.T) {
	source := `<?php
namespace App;

class Demo {
    public function run(): void {
        new Logger();
    }
}
`

	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///test.php": source,
		"file:///vendor/logger.php": `<?php
namespace Monolog;
class Logger {}
`,
		"file:///vendor/logger2.php": `<?php
namespace Psr\Log;
interface Logger {}
`,
	})

	diag := unknownClassDiagnostic(5, 12, 18, "Unknown class 'Logger'")
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{diag}, Only: []string{"quickfix"}},
	})

	if len(actions) != 2 {
		t.Fatalf("expected two deterministic quickfixes, got %d: %#v", len(actions), actions)
	}
	if actions[0].Title != "Import Monolog\\Logger" || actions[1].Title != "Import Psr\\Log\\Logger" {
		t.Fatalf("unexpected quickfix order: %#v", actions)
	}
	if actions[0].IsPreferred || actions[1].IsPreferred {
		t.Fatal("did not expect ambiguous quickfixes to be preferred")
	}
}

func TestGetCodeActionsUnknownClassShortensQualifiedReferenceWhenSafe(t *testing.T) {
	source := `<?php
namespace App;

class Demo {
    public function run(): void {
        new Handler\StreamHandler();
    }
}
`

	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///test.php": source,
		"file:///vendor/stream_handler.php": `<?php
namespace Monolog\Handler;
class StreamHandler {}
`,
	})

	diag := unknownClassDiagnostic(5, 12, 33, "Unknown class 'Handler\\StreamHandler'")
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{diag}, Only: []string{"quickfix"}},
	})

	if len(actions) != 1 {
		t.Fatalf("expected one quickfix, got %d: %#v", len(actions), actions)
	}
	edits := actions[0].Edit.Changes["file:///test.php"]
	if len(edits) != 2 {
		t.Fatalf("expected import + replacement edits, got %#v", edits)
	}
	if edits[0].NewText != "use Monolog\\Handler\\StreamHandler;\n" {
		t.Fatalf("unexpected import edit %q", edits[0].NewText)
	}
	if edits[1].NewText != "StreamHandler" {
		t.Fatalf("unexpected replacement text %q", edits[1].NewText)
	}
	if edits[1].Range.Start.Character != 12 || edits[1].Range.End.Character != 33 {
		t.Fatalf("unexpected replacement range %#v", edits[1].Range)
	}
}

func TestGetCodeActionsUnknownClassSkipsConflictingImportAlias(t *testing.T) {
	source := `<?php
namespace App;

use Other\Logger;

class Demo {
    public function run(): void {
        new Monolog\Logger();
    }
}
`

	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///test.php": source,
		"file:///vendor/logger.php": `<?php
namespace Monolog;
class Logger {}
`,
	})

	diag := unknownClassDiagnostic(7, 12, 26, "Unknown class 'Monolog\\Logger'")
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{diag}, Only: []string{"quickfix"}},
	})

	if len(actions) != 0 {
		t.Fatalf("expected no quickfix for conflicting alias, got %#v", actions)
	}
}

func TestGetCodeActionsRespectsOnlyFilter(t *testing.T) {
	source := `<?php
namespace App\Models;
class User {}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Context:      protocol.CodeActionContext{Only: []string{"quickfix"}},
	})

	for _, action := range actions {
		if strings.HasPrefix(action.Title, "Copy Namespace:") {
			t.Fatalf("did not expect source action when only quickfix requested: %#v", actions)
		}
	}
}
