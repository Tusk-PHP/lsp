package diagnostics

import (
	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// ResolveTypes turns on the type-aware rules (unknown members, argument
// counts, named arguments, builder columns and relations and the like): it
// sets TypeResolver and BuilderModelResolver to the engine's own type
// resolution over p's index, with container calls typed from ca when it
// isn't nil.
func (p *Provider) ResolveTypes(ca *container.ContainerAnalyzer) {
	engine := container.NewEngine(p.index, ca)
	p.TypeResolver = func(expr, source string, line, col int, file *parser.FileNode) string {
		return engine.DiagnosticType(expr, source, textpos.Position{Line: line, Character: col}, file)
	}
	p.BuilderModelResolver = func(prefix, method, source string, line int, file *parser.FileNode) frameworklaravel.QueriedModel {
		return frameworklaravel.BuilderModel(engine.Resolver(), p.index, method, source, textpos.Position{Line: line, Character: len(prefix)}, file)
	}
}
