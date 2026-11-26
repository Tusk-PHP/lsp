package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/rule"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (a *Analyzer) unknownClassCodeActions(uri, source string, tree *ast.File, file *parser.FileNode, params protocol.CodeActionParams) []protocol.CodeAction {
	if !protocol.CodeActionQuickFix.In(params.Context.Only) {
		return nil
	}

	var actions []protocol.CodeAction
	var namespaces *sourcectx.Namespaces
	seen := make(map[string]bool)

	for _, diag := range params.Context.Diagnostics {
		if diag.Code != string(rule.UnknownClass) {
			continue
		}

		refText, replaceRange, ok := unknownClassReferenceAt(source, diag.Range)
		if !ok {
			continue
		}

		candidates := a.lookupUnknownClassCandidates(refText)
		preferred := len(candidates) == 1
		if namespaces == nil {
			namespaces = sourcectx.NamespacesOf(source, ast.NewLines(source), tree)
		}
		offset := namespaces.Offset(diag.Range.Start)
		view, site := namespaces.At(offset).View(file), namespaces.UseSiteAt(offset)

		for _, candidate := range candidates {
			action, ok := buildUnknownClassImportAction(uri, view, site, diag, refText, replaceRange, candidate, preferred)
			if !ok {
				continue
			}
			key := action.Title
			if action.Edit != nil {
				key += fmt.Sprintf("%#v", action.Edit.Changes[uri])
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			actions = append(actions, action)
		}
	}

	return actions
}

func (a *Analyzer) lookupUnknownClassCandidates(refText string) []*symbols.Symbol {
	name := strings.TrimPrefix(refText, "\\")
	if name == "" {
		return nil
	}

	shortName := name
	if idx := strings.LastIndex(shortName, "\\"); idx >= 0 {
		shortName = shortName[idx+1:]
	}

	var candidates []*symbols.Symbol
	for _, sym := range a.index.LookupByName(shortName) {
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
			candidates = append(candidates, sym)
		}
	}

	if strings.Contains(name, "\\") {
		var narrowed []*symbols.Symbol
		for _, candidate := range candidates {
			if candidate.FQN == name || strings.HasSuffix(candidate.FQN, "\\"+name) {
				narrowed = append(narrowed, candidate)
			}
		}
		if len(narrowed) > 0 {
			candidates = narrowed
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].FQN < candidates[j].FQN
	})

	deduped := candidates[:0]
	var last string
	for _, candidate := range candidates {
		if candidate == nil || candidate.FQN == "" || candidate.FQN == last {
			continue
		}
		deduped = append(deduped, candidate)
		last = candidate.FQN
	}

	return deduped
}

func buildUnknownClassImportAction(uri string, file *parser.FileNode, site sourcectx.UseSite, diag protocol.Diagnostic, refText string, replaceRange protocol.Range, candidate *symbols.Symbol, preferred bool) (protocol.CodeAction, bool) {
	if file == nil || candidate == nil || candidate.FQN == "" || candidate.Name == "" {
		return protocol.CodeAction{}, false
	}
	if !canImportClassName(file, candidate.Name, candidate.FQN) {
		return protocol.CodeAction{}, false
	}

	edits := []protocol.TextEdit{site.Insert("use " + candidate.FQN + ";\n")}
	if shouldShortenUnknownClassReference(file, refText, candidate.Name, candidate.FQN) {
		edits = append(edits, protocol.TextEdit{
			Range:   replaceRange,
			NewText: candidate.Name,
		})
	}

	return protocol.CodeAction{
		Title:       "Import " + candidate.FQN,
		Kind:        protocol.CodeActionQuickFix,
		Diagnostics: []protocol.Diagnostic{diag},
		IsPreferred: preferred,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[string][]protocol.TextEdit{
				uri: edits,
			},
		},
	}, true
}

func unknownClassReferenceAt(source string, diagRange protocol.Range) (string, protocol.Range, bool) {
	lines := strings.Split(source, "\n")
	lineNo := diagRange.Start.Line
	if lineNo < 0 || lineNo >= len(lines) {
		return "", protocol.Range{}, false
	}

	line := lines[lineNo]
	start := diagRange.Start.Character
	end := diagRange.End.Character
	if start < 0 || end < start || end > len(line) {
		return "", protocol.Range{}, false
	}

	if start > 0 && line[start-1] == '\\' {
		start--
	}

	if start == end {
		return "", protocol.Range{}, false
	}

	return line[start:end], protocol.Range{
		Start: protocol.Position{Line: lineNo, Character: start},
		End:   protocol.Position{Line: lineNo, Character: end},
	}, true
}

func canImportClassName(file *parser.FileNode, shortName, fqn string) bool {
	for _, useStmt := range file.Uses {
		if useStmt.Kind == "function" || useStmt.Kind == "const" {
			continue
		}
		if useStmt.Alias != shortName {
			continue
		}
		return false
	}

	for _, cls := range file.Classes {
		if cls.Name == shortName && classLikeNodeFQN(file.Namespace, cls.FullName, cls.Name) != fqn {
			return false
		}
	}
	for _, iface := range file.Interfaces {
		if iface.Name == shortName && classLikeNodeFQN(file.Namespace, iface.FullName, iface.Name) != fqn {
			return false
		}
	}
	for _, tr := range file.Traits {
		if tr.Name == shortName && classLikeNodeFQN(file.Namespace, tr.FullName, tr.Name) != fqn {
			return false
		}
	}
	for _, en := range file.Enums {
		if en.Name == shortName && classLikeNodeFQN(file.Namespace, en.FullName, en.Name) != fqn {
			return false
		}
	}

	return true
}

func shouldShortenUnknownClassReference(file *parser.FileNode, refText, shortName, fqn string) bool {
	name := strings.TrimPrefix(refText, "\\")
	if name == "" || !strings.Contains(name, "\\") {
		return false
	}
	return canImportClassName(file, shortName, fqn)
}

func classLikeNodeFQN(namespace, fullName, name string) string {
	if fullName != "" {
		return fullName
	}
	if namespace != "" {
		return namespace + "\\" + name
	}
	return name
}
