package analyzer

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

type importCodeActionLine struct {
	useStmt parser.UseNode
	text    string
}

type importCodeActionBlock struct {
	startLine int
	endLine   int
	lines     []importCodeActionLine
}

func (a *Analyzer) importUnusedImportQuickFixes(uri, source string, params protocol.CodeActionParams) []protocol.CodeAction {
	if !protocol.CodeActionQuickFix.In(params.Context.Only) {
		return nil
	}

	lines := strings.Split(source, "\n")
	if len(lines) == 0 {
		return nil
	}

	seen := make(map[int]bool)
	var actions []protocol.CodeAction
	for _, diag := range params.Context.Diagnostics {
		if diag.Code != string(rule.UnusedImport) {
			continue
		}

		line := diag.Range.Start.Line
		if line < 0 || line >= len(lines) || seen[line] || !isSimpleImportUseLine(lines[line]) {
			continue
		}
		seen[line] = true

		actions = append(actions, protocol.CodeAction{
			Title:       "Remove unused import",
			Kind:        protocol.CodeActionQuickFix,
			Diagnostics: []protocol.Diagnostic{diag},
			IsPreferred: true,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[string][]protocol.TextEdit{
					uri: {{
						Range:   importWholeLineRange(lines, line),
						NewText: "",
					}},
				},
			},
		})
	}

	return actions
}

func (a *Analyzer) importOrganizeImportsAction(uri string, result *parser.ParseResult, file *parser.FileNode, only []protocol.CodeActionKind) *protocol.CodeAction {
	if !protocol.CodeActionSourceOrganizeImports.In(only) {
		return nil
	}

	lines := result.Lines
	block, ok := safeImportCodeActionBlock(lines, file)
	if !ok {
		return nil
	}

	refs := result.ImportReferences()
	kept := make([]importCodeActionLine, 0, len(block.lines))
	for _, line := range block.lines {
		if refs.Used(line.useStmt) {
			kept = append(kept, line)
		}
	}

	sort.SliceStable(kept, func(i, j int) bool {
		left, right := kept[i], kept[j]
		if importKindOrder(left.useStmt.Kind) != importKindOrder(right.useStmt.Kind) {
			return importKindOrder(left.useStmt.Kind) < importKindOrder(right.useStmt.Kind)
		}
		if !strings.EqualFold(left.useStmt.FullName, right.useStmt.FullName) {
			return strings.ToLower(left.useStmt.FullName) < strings.ToLower(right.useStmt.FullName)
		}
		if !strings.EqualFold(left.useStmt.Alias, right.useStmt.Alias) {
			return strings.ToLower(left.useStmt.Alias) < strings.ToLower(right.useStmt.Alias)
		}
		return left.text < right.text
	})

	replacement := renderImportCodeActionBlock(kept)
	current := strings.Join(lines[block.startLine:block.endLine+1], "\n")
	if replacement == current {
		return nil
	}

	return &protocol.CodeAction{
		Title: "Organize Imports",
		Kind:  protocol.CodeActionSourceOrganizeImports,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[string][]protocol.TextEdit{
				uri: {{
					Range: protocol.Range{
						Start: protocol.Position{Line: block.startLine, Character: 0},
						End:   importBlockReplaceEnd(lines, block.endLine),
					},
					NewText: replacement,
				}},
			},
		},
	}
}

func safeImportCodeActionBlock(lines []string, file *parser.FileNode) (importCodeActionBlock, bool) {
	if file == nil || len(file.Uses) == 0 {
		return importCodeActionBlock{}, false
	}

	block := importCodeActionBlock{
		startLine: file.Uses[0].StartLine,
		endLine:   file.Uses[len(file.Uses)-1].StartLine,
		lines:     make([]importCodeActionLine, 0, len(file.Uses)),
	}
	if block.startLine < 0 || block.endLine >= len(lines) || block.startLine > block.endLine {
		return importCodeActionBlock{}, false
	}

	useByLine := make(map[int]parser.UseNode, len(file.Uses))
	for _, useStmt := range file.Uses {
		if useStmt.StartLine < 0 || useStmt.StartLine >= len(lines) {
			return importCodeActionBlock{}, false
		}
		if !isSimpleImportUseLine(lines[useStmt.StartLine]) {
			return importCodeActionBlock{}, false
		}
		useByLine[useStmt.StartLine] = useStmt
	}

	for lineNo := block.startLine; lineNo <= block.endLine; lineNo++ {
		line := lines[lineNo]
		if useStmt, ok := useByLine[lineNo]; ok {
			block.lines = append(block.lines, importCodeActionLine{
				useStmt: useStmt,
				text:    strings.TrimSpace(line),
			})
			continue
		}
		if strings.TrimSpace(line) != "" {
			return importCodeActionBlock{}, false
		}
	}

	return block, true
}

func renderImportCodeActionBlock(lines []importCodeActionLine) string {
	if len(lines) == 0 {
		return ""
	}

	var b strings.Builder
	lastKind := ""
	for i, line := range lines {
		if i > 0 {
			if line.useStmt.Kind != lastKind {
				b.WriteString("\n\n")
			} else {
				b.WriteByte('\n')
			}
		}
		b.WriteString(line.text)
		lastKind = line.useStmt.Kind
	}

	return b.String()
}

func importBlockReplaceEnd(lines []string, endLine int) protocol.Position {
	if endLine+1 < len(lines) {
		return protocol.Position{Line: endLine + 1, Character: 0}
	}
	return protocol.Position{Line: endLine, Character: len(lines[endLine])}
}

func importWholeLineRange(lines []string, line int) protocol.Range {
	end := protocol.Position{Line: line, Character: len(lines[line])}
	if line+1 < len(lines) {
		end = protocol.Position{Line: line + 1, Character: 0}
	}
	return protocol.Range{
		Start: protocol.Position{Line: line, Character: 0},
		End:   end,
	}
}

func isSimpleImportUseLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "use ") &&
		strings.HasSuffix(trimmed, ";") &&
		!strings.Contains(trimmed, "{") &&
		!strings.Contains(trimmed, ",")
}

func importKindOrder(kind string) int {
	switch kind {
	case "function":
		return 1
	case "const":
		return 2
	default:
		return 0
	}
}
