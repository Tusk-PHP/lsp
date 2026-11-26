package checks

import (
	"fmt"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// UnusedImportsRule detects use statements whose imported symbol is never
// referenced in the file.
type UnusedImportsRule struct {
	Shared *Shared
}

func (r *UnusedImportsRule) Codes() []rule.Code { return []rule.Code{rule.UnusedImport} }

func (r *UnusedImportsRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil || len(file.Uses) == 0 {
		return nil
	}
	result := r.Shared.parse(source)
	refs := result.ImportReferences()

	var findings []Finding
	for _, u := range file.Uses {
		if refs.Used(u) {
			continue
		}

		endCol := 0
		if u.StartLine >= 0 && u.StartLine < len(result.Lines) {
			endCol = len(result.Lines[u.StartLine])
		}

		findings = append(findings, Finding{
			StartLine: u.StartLine,
			StartCol:  0,
			EndLine:   u.StartLine,
			EndCol:    endCol,
			Severity:  rule.SeverityHint,
			Code:      rule.UnusedImport,
			Message:   fmt.Sprintf("Unused import '%s'", u.FullName),
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}
