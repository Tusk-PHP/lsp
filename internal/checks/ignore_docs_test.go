package checks

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

func TestDiagnosticsDocsIgnoreExampleMatchesARealFinding(t *testing.T) {
	doc, err := os.ReadFile("../../docs/diagnostics.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(doc), "```json\n{\n  \"diagnosticIgnore\"")
	if !ok {
		t.Fatal("docs/diagnostics.md has no diagnosticIgnore example")
	}
	example, _, _ := strings.Cut("{\n  \"diagnosticIgnore\""+rest, "```")
	var cfg config.Config
	if err := json.Unmarshal([]byte(example), &cfg); err != nil || len(cfg.Problems()) != 0 {
		t.Fatalf("the example loads cleanly: %v %v", err, cfg.Problems())
	}
	model := "<?php\nnamespace App\\Models;\nclass LegacyModel {}\n"
	findings := checkUnknownMembers(t, map[string]string{"$v": `App\Models\LegacyModel`}, model, "<?php\n$v->missing();\n")
	if len(findings) != 1 {
		t.Fatalf("want one finding, got %v", findings)
	}
	f := findings[0]
	for _, r := range cfg.DiagnosticIgnore.Rules {
		if r.Matches(string(rule.SourceTusk), string(f.Code), f.Message) && r.MatchesPath("app/Http/Controller.php") {
			return
		}
	}
	t.Errorf("no rule in the example ignores %q", f.Message)
}
