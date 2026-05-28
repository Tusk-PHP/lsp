package analyzer

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
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
	if !codeActionKindAllowed(params.Context.Only, "quickfix") {
		return nil
	}

	lines := strings.Split(source, "\n")
	if len(lines) == 0 {
		return nil
	}

	seen := make(map[int]bool)
	var actions []protocol.CodeAction
	for _, diag := range params.Context.Diagnostics {
		if diag.Code != "unused-import" {
			continue
		}

		line := diag.Range.Start.Line
		if line < 0 || line >= len(lines) || seen[line] || !isSimpleImportUseLine(lines[line]) {
			continue
		}
		seen[line] = true

		actions = append(actions, protocol.CodeAction{
			Title:       "Remove unused import",
			Kind:        "quickfix",
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

func (a *Analyzer) importOrganizeImportsAction(uri, source string, file *parser.FileNode, only []string) *protocol.CodeAction {
	if !codeActionKindAllowed(only, "source.organizeImports") {
		return nil
	}

	block, ok := safeImportCodeActionBlock(source, file)
	if !ok {
		return nil
	}

	lines := strings.Split(source, "\n")
	kept := make([]importCodeActionLine, 0, len(block.lines))
	for _, line := range block.lines {
		if importCodeActionUsed(line.useStmt, lines) {
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
		Kind:  "source.organizeImports",
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

func safeImportCodeActionBlock(source string, file *parser.FileNode) (importCodeActionBlock, bool) {
	if file == nil || len(file.Uses) == 0 {
		return importCodeActionBlock{}, false
	}

	lines := strings.Split(source, "\n")
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

func importCodeActionUsed(u parser.UseNode, lines []string) bool {
	alias := u.Alias
	if alias == "" {
		if idx := strings.LastIndex(u.FullName, "\\"); idx >= 0 {
			alias = u.FullName[idx+1:]
		} else {
			alias = u.FullName
		}
	}

	for i, line := range lines {
		if i == u.StartLine {
			continue
		}
		if containsImportCodeActionWord(line, alias) {
			return true
		}
	}
	return false
}

func containsImportCodeActionWord(line, name string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, name) {
		return false
	}

	start := 0
	for {
		idx := strings.Index(line[start:], name)
		if idx < 0 {
			return false
		}
		absIdx := start + idx
		before := absIdx - 1
		after := absIdx + len(name)

		leftOK := before < 0 || !isImportCodeActionIdentChar(line[before])
		rightOK := after >= len(line) || !isImportCodeActionIdentChar(line[after])
		if leftOK && rightOK {
			return true
		}

		start = absIdx + 1
		if start >= len(line) {
			return false
		}
	}
}

func isImportCodeActionIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}
