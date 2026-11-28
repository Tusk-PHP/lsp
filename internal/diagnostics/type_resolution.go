package diagnostics

import (
	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

// ResolveTypes turns on the type-aware rules (unknown members, argument
// counts, named arguments, builder columns and relations and the like): it
// sets TypeResolver and BuilderModelResolver to the engine's own type
// resolution over p's index, with container calls typed from ca when it
// isn't nil.
func (p *Provider) ResolveTypes(ca *container.ContainerAnalyzer) {
	engine := container.NewEngine(p.index, ca)
	p.TypeResolver = engine.DiagnosticType
	p.BuilderModelResolver = func(file *parser.FileNode, receiver ast.Expr) frameworklaravel.QueriedModel {
		return frameworklaravel.BuilderModel(engine.Resolver(), p.index, file, receiver)
	}
}
