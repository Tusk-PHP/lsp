package analyzer

import (
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
)

func (a *Analyzer) authNameDefinition(r *resolve.Resolver, file *parser.FileNode, name frameworklaravel.AuthName) *protocol.Location {
	if name.Kind == frameworklaravel.AbilityName {
		ability, ok := a.authorization.AbilityOf(r, file, name)
		if !ok {
			return nil
		}
		return &protocol.Location{URI: ability.URI, Range: ability.Range}
	}
	registered := a.authorization.Middleware(name.Name)
	if len(registered) == 0 {
		return nil
	}
	m := registered[0]
	if m.Class != "" {
		if l := symbolLocation(a.index.Lookup(m.Class)); l != nil {
			return l
		}
	}
	return &protocol.Location{URI: m.URI, Range: m.Range}
}
