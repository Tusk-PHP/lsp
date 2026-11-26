package completion

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/container"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func isContainerArg(arg sourcectx.CallArg, src string) bool {
	if arg.Index != 0 || arg.InArray {
		return false
	}
	if slices.Contains(container.ResolvingFunctions, arg.Function) {
		return true
	}
	if arg.Static || !slices.Contains(container.ResolvingMethods, arg.Method) {
		return false
	}
	switch recv := arg.Receiver.(type) {
	case *ast.Variable:
		return recv.Bounds().Text(src) == "$container"
	case *ast.PropertyFetch:
		this, ok := recv.Object.(*ast.Variable)
		return arg.Method == "make" && ok && this.Bounds().Text(src) == "$this" && recv.Name.Bounds().Text(src) == "app"
	}
	return false
}

// completeContainerResolve returns completion items for container resolution calls.
// quoteCtx is the opening quote character already typed ("'" or "\""), or empty
// if the user hasn't typed a quote yet. String bindings get wrapped in quotes
// accordingly.
func (p *Provider) completeContainerResolve(source, filter, quoteCtx string, file *parser.FileNode, rk nameRanker) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	lfilter := strings.ToLower(filter)

	q := "'"
	if quoteCtx == "\"" {
		q = "\""
	}

	if p.container != nil {
		for abstract, binding := range p.container.GetBindings() {
			if lfilter != "" && !strings.HasPrefix(strings.ToLower(abstract), lfilter) {
				parts := strings.Split(abstract, "\\")
				shortName := parts[len(parts)-1]
				if !strings.HasPrefix(strings.ToLower(shortName), lfilter) {
					continue
				}
			}
			resolved := p.container.ResolveDependency(abstract)
			if resolved == nil {
				resolved = binding
			}
			d := fmt.Sprintf("-> %s", resolved.Concrete)
			if binding.Alias != "" {
				d += fmt.Sprintf(" (alias of %s)", binding.Alias)
			}
			if resolved.Singleton {
				d += " (singleton)"
			}
			label := abstract
			sortText := "1" + abstract

			var insertText string
			if !strings.Contains(abstract, "\\") {
				sortText = "0" + abstract
			}
			if quoteCtx != "" {
				insertText = abstract
			} else {
				insertText = q + abstract + q
			}

			items = append(items, protocol.CompletionItem{
				Label:      label,
				Kind:       protocol.CompletionItemKindValue,
				Detail:     d,
				InsertText: insertText,
				SortText:   sortText,
			})
		}
	}

	for _, sym := range p.index.SearchByPrefix(filter) {
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum:
			if p.container != nil {
				if _, bound := p.container.GetBindings()[sym.FQN]; bound {
					continue
				}
			}
			insertName := sym.FQN
			if file != nil {
				for _, u := range file.Uses {
					if u.FullName == sym.FQN {
						insertName = u.Alias
						break
					}
				}
			}
			classInsert := insertName + "::class"
			if quoteCtx != "" {
				classInsert = insertName + "::class"
			}
			items = append(items, protocol.CompletionItem{
				Label:      sym.Name + "::class",
				Kind:       protocol.CompletionItemKindClass,
				Detail:     sym.FQN,
				InsertText: classInsert,
				SortText:   rk.key(sym, sym.Name),
			})
		}
	}

	return items
}
