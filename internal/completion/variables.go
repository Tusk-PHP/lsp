package completion

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

var superglobals = []string{"$GLOBALS", "$_COOKIE", "$_ENV", "$_FILES", "$_GET", "$_POST", "$_REQUEST", "$_SERVER", "$_SESSION"}

// typedVariablePrefix returns the variable name being typed at the end of
// prefix, such as "$us" in `foo($us`, including the `$`.
func typedVariablePrefix(prefix string) (string, bool) {
	i := len(prefix)
	for i > 0 && resolve.IsWordChar(prefix[i-1]) {
		i--
	}
	if i == 0 || prefix[i-1] != '$' {
		return "", false
	}
	// `$$name` is a variable variable, and `::$name` a static property.
	if i >= 2 && (prefix[i-2] == '$' || prefix[i-2] == ':') {
		return "", false
	}
	return prefix[i-1:], true
}

// completeVariables lists the variables visible at pos, closest first: the
// function's own variables, most recently used first, then its parameters in
// order, then $this, then PHP's superglobals.
func (p *Provider) completeVariables(typed string, lines []string, pos protocol.Position, file *parser.FileNode) []protocol.CompletionItem {
	ltyped := strings.ToLower(typed)
	matches := func(name string) bool { return strings.HasPrefix(strings.ToLower(name), ltyped) }

	var items []protocol.CompletionItem
	seen := map[string]bool{}
	add := func(name, detail string, tier, sub int) {
		if seen[name] || !matches(name) || name == typed {
			return
		}
		seen[name] = true
		items = append(items, protocol.CompletionItem{
			Label:    name,
			Kind:     protocol.CompletionItemKindVariable,
			Detail:   detail,
			SortText: rankKey(tier, sub, name),
		})
	}

	doc := p.resolver.ScopeDocument(lines)
	s := doc.ScopeAt(pos)
	// An arrow function also sees its parent's variables; they rank after its own.
	for depth := 0; s != nil; depth++ {
		for _, b := range s.Bindings {
			if !strings.HasPrefix(b.Name, "$") || b.Kind == scope.BindingImport {
				continue
			}
			if b.Kind == scope.BindingParameter {
				add(b.Name, "parameter", rankParameter, depth*1000+b.Decl.Start.Line*200+b.Decl.Start.Character)
				continue
			}
			last, ok := lastUseBefore(b, pos)
			if !ok {
				continue
			}
			add(b.Name, "variable", rankLocal, depth*10000+pos.Line-last)
		}
		if !s.ExprBody {
			break
		}
		s = s.Parent
	}

	if file != nil && resolve.FindEnclosingClass(file, pos) != "" {
		if m := resolve.FindEnclosingMethod(file, pos); m == nil || !m.IsStatic {
			add("$this", "Current object instance", rankMember, 0)
		}
	}
	for _, g := range superglobals {
		add(g, "superglobal", rankLanguage, 0)
	}
	return items
}

// lastUseBefore returns the line of the latest use of b above pos (assigned,
// updated or read), or false when it isn't assigned above pos, so it can't be
// used yet.
func lastUseBefore(b *scope.Binding, pos protocol.Position) (int, bool) {
	before := func(r protocol.Range) bool {
		return r.End.Line < pos.Line || (r.End.Line == pos.Line && r.End.Character < pos.Character)
	}
	line, assigned := -1, false
	for _, w := range append([]protocol.Range{b.Decl}, b.Writes...) {
		if before(w) {
			assigned = true
			line = max(line, w.Start.Line)
		}
	}
	if !assigned {
		return 0, false
	}
	for _, r := range b.References {
		if before(r) {
			line = max(line, r.Start.Line)
		}
	}
	return line, true
}
