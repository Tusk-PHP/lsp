package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func autoImportIndex(t *testing.T) *symbols.Index {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///app/Models/User.php", "<?php\nnamespace App\\Models;\nclass User {}\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///vendor/Auth/User.php", "<?php\nnamespace Illuminate\\Foundation\\Auth;\nclass User {}\n", symbols.SourceVendor)
	idx.IndexFileWithSource("file:///app/Http/Controllers/Report.php", "<?php\nnamespace App\\Http\\Controllers;\nclass Report {}\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///app/Reports/Report.php", "<?php\nnamespace App\\Reports;\nclass Report {}\n", symbols.SourceProject)
	return idx
}

// completeAt completes at the end of source and returns the item for fqn.
func completeAt(t *testing.T, p *Provider, source, fqn string) protocol.CompletionItem {
	t.Helper()
	lines := strings.Split(source, "\n")
	pos := protocol.Position{Line: len(lines) - 1, Character: len(lines[len(lines)-1])}
	for _, item := range p.GetCompletions("file:///app/Http/Controllers/HomeController.php", source, pos) {
		if item.Detail == fqn {
			return item
		}
	}
	t.Fatalf("no completion item for %s", fqn)
	return protocol.CompletionItem{}
}

func insertedName(item protocol.CompletionItem) string {
	if item.InsertText != "" {
		return item.InsertText
	}
	return item.Label
}

func importText(item protocol.CompletionItem) string {
	var b strings.Builder
	for _, e := range item.AdditionalTextEdits {
		b.WriteString(e.NewText)
	}
	return b.String()
}

const controllerHead = "<?php\nnamespace App\\Http\\Controllers;\n\n"

func TestAutoImportAliasesClashWithImport(t *testing.T) {
	p := NewProvider(autoImportIndex(t), nil, "")
	src := controllerHead + "use App\\Models\\User;\n\nclass HomeController\n{\n    public function index()\n    {\n        User"
	item := completeAt(t, p, src, "Illuminate\\Foundation\\Auth\\User")
	if got := importText(item); got != "use Illuminate\\Foundation\\Auth\\User as UserAuth;\n" {
		t.Errorf("import = %q, want an aliased import", got)
	}
	if got := insertedName(item); got != "UserAuth" {
		t.Errorf("inserted %q, want UserAuth", got)
	}
	// The class already imported keeps its name and adds nothing.
	item = completeAt(t, p, src, "App\\Models\\User")
	if got := importText(item); got != "" {
		t.Errorf("already imported class added %q", got)
	}
	if got := insertedName(item); got != "User" {
		t.Errorf("inserted %q, want User", got)
	}
}

func TestAutoImportAliasesClashIgnoringCase(t *testing.T) {
	p := NewProvider(autoImportIndex(t), nil, "")
	src := controllerHead + "use Some\\Other\\USER;\n\nclass HomeController\n{\n    public function index()\n    {\n        new User"
	item := completeAt(t, p, src, "App\\Models\\User")
	if got := importText(item); got != "use App\\Models\\User as UserModel;\n" {
		t.Errorf("import = %q", got)
	}
	if item.InsertText != "UserModel($0)" {
		t.Errorf("insert = %q, want UserModel($0)", item.InsertText)
	}
}

func TestAutoImportAliasesClashWithDeclaredClass(t *testing.T) {
	p := NewProvider(autoImportIndex(t), nil, "")
	src := "<?php\nnamespace App\\Http\\Controllers;\n\nclass User\n{\n    public function index()\n    {\n        User"
	item := completeAt(t, p, src, "App\\Models\\User")
	if got := importText(item); got != "use App\\Models\\User as UserModel;\n\n" && got != "use App\\Models\\User as UserModel;\n" {
		t.Errorf("import = %q", got)
	}
	if got := insertedName(item); got != "UserModel" {
		t.Errorf("inserted %q, want UserModel", got)
	}
}

func TestAutoImportAliasesClashWithSameNamespaceClass(t *testing.T) {
	p := NewProvider(autoImportIndex(t), nil, "")
	src := controllerHead + "class HomeController\n{\n    public function index()\n    {\n        Report"
	item := completeAt(t, p, src, "App\\Reports\\Report")
	if got := importText(item); got != "use App\\Reports\\Report as ReportReport;\n" {
		t.Errorf("import = %q", got)
	}
	// The class in the file's own namespace needs no import.
	item = completeAt(t, p, src, "App\\Http\\Controllers\\Report")
	if got := importText(item); got != "" || insertedName(item) != "Report" {
		t.Errorf("same-namespace class: import %q, insert %q", got, insertedName(item))
	}
}

func TestAutoImportPlainWhenNameIsFree(t *testing.T) {
	p := NewProvider(autoImportIndex(t), nil, "")
	src := controllerHead + "use App\\Models\\Post;\n\nclass HomeController\n{\n    public function index()\n    {\n        User"
	item := completeAt(t, p, src, "App\\Models\\User")
	if got := importText(item); got != "use App\\Models\\User;\n" {
		t.Errorf("import = %q", got)
	}
	if got := insertedName(item); got != "User" {
		t.Errorf("inserted %q, want User", got)
	}
	if line := item.AdditionalTextEdits[0].Range.Start.Line; line != 4 {
		t.Errorf("import on line %d, want 4 (after the last use)", line)
	}
}

func TestAutoImportAliasStyles(t *testing.T) {
	src := controllerHead + "use App\\Models\\User;\n\nclass HomeController\n{\n    public function index()\n    {\n        User"
	tests := []struct {
		style, insert, use string
	}{
		{"", "UserAuth", "use Illuminate\\Foundation\\Auth\\User as UserAuth;\n"},
		{AliasStyleSuffix, "UserAuth", "use Illuminate\\Foundation\\Auth\\User as UserAuth;\n"},
		{AliasStylePrefix, "AuthUser", "use Illuminate\\Foundation\\Auth\\User as AuthUser;\n"},
		{AliasStyleFullyQualified, "\\Illuminate\\Foundation\\Auth\\User", ""},
		{"bogus", "UserAuth", "use Illuminate\\Foundation\\Auth\\User as UserAuth;\n"},
	}
	for _, tt := range tests {
		p := NewProvider(autoImportIndex(t), nil, "")
		p.SetAutoImportAliasStyle(tt.style)
		item := completeAt(t, p, src, "Illuminate\\Foundation\\Auth\\User")
		if got := insertedName(item); got != tt.insert {
			t.Errorf("style %q: inserted %q, want %q", tt.style, got, tt.insert)
		}
		if got := importText(item); got != tt.use {
			t.Errorf("style %q: import %q, want %q", tt.style, got, tt.use)
		}
	}
}

func TestAutoImportAliasTakenAddsNextSegment(t *testing.T) {
	p := NewProvider(autoImportIndex(t), nil, "")
	src := controllerHead + "use App\\Models\\User;\nuse Other\\UserAuth;\n\nclass HomeController\n{\n    public function index()\n    {\n        User"
	item := completeAt(t, p, src, "Illuminate\\Foundation\\Auth\\User")
	if got := importText(item); got != "use Illuminate\\Foundation\\Auth\\User as UserFoundationAuth;\n" {
		t.Errorf("import = %q", got)
	}

	p.SetAutoImportAliasStyle(AliasStylePrefix)
	src = controllerHead + "use App\\Models\\User;\nuse Other\\AuthUser;\n\nclass HomeController\n{\n    public function index()\n    {\n        User"
	item = completeAt(t, p, src, "Illuminate\\Foundation\\Auth\\User")
	if got := importText(item); got != "use Illuminate\\Foundation\\Auth\\User as FoundationAuthUser;\n" {
		t.Errorf("prefix import = %q", got)
	}
}

func TestAutoImportFallsBackToFullyQualified(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")
	file := parser.ParseFile("<?php\nnamespace App;\nuse Other\\User;\nuse Other\\UserModel;\n")
	name, edits := p.newImportPlanner("", file).class("Models\\User")
	if name != "\\Models\\User" || edits != nil {
		t.Errorf("got %q %v, want \\Models\\User with no import", name, edits)
	}
}

func TestAutoImportFunctionsAndConstantsHaveTheirOwnNames(t *testing.T) {
	p := NewProvider(symbols.NewIndex(), nil, "")
	src := "<?php\nnamespace App;\nuse Foo\\Bar\\User;\nuse function Foo\\Helpers\\format;\nuse const Foo\\Codes\\LIMIT;\n"
	pl := p.newImportPlanner(src, parser.ParseFile(src))

	// A class import doesn't take a function's name.
	if name, edits := pl.plan(importFunction, "Lib\\Helpers\\user"); name != "user" || importTextOf(edits) != "use function Lib\\Helpers\\user;\n" {
		t.Errorf("function: %q %q", name, importTextOf(edits))
	}
	if name, edits := pl.plan(importFunction, "Lib\\Helpers\\Format"); name != "FormatHelper" || importTextOf(edits) != "use function Lib\\Helpers\\Format as FormatHelper;\n" {
		t.Errorf("clashing function: %q %q", name, importTextOf(edits))
	}
	if name, edits := pl.plan(importConst, "Lib\\Limits\\LIMIT"); name != "LIMITLimit" || importTextOf(edits) != "use const Lib\\Limits\\LIMIT as LIMITLimit;\n" {
		t.Errorf("clashing const: %q %q", name, importTextOf(edits))
	}
}

func importTextOf(edits []protocol.TextEdit) string {
	return importText(protocol.CompletionItem{AdditionalTextEdits: edits})
}

func TestSingularize(t *testing.T) {
	for in, want := range map[string]string{
		"Models":      "Model",
		"Categories":  "Category",
		"Statuses":    "Status",
		"Boxes":       "Box",
		"Matches":     "Match",
		"Hashes":      "Hash",
		"Classes":     "Class",
		"Resources":   "Resource",
		"Responses":   "Response",
		"Requests":    "Request",
		"Status":      "Status",
		"Analysis":    "Analysis",
		"Class":       "Class",
		"Auth":        "Auth",
		"Http":        "Http",
		"Middleware":  "Middleware",
		"Foundation":  "Foundation",
		"Controllers": "Controller",
	} {
		if got := singularize(in); got != want {
			t.Errorf("singularize(%q) = %q, want %q", in, got, want)
		}
	}
}
