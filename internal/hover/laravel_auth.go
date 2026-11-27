package hover

import (
	"path"
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
)

func (p *Provider) hoverAuthName(r *resolve.Resolver, file *parser.FileNode, name frameworklaravel.AuthName) *protocol.Hover {
	var b strings.Builder
	if name.Kind == frameworklaravel.AbilityName {
		abilities := p.authorization.Abilities(name.Name)
		if len(abilities) == 0 {
			return nil
		}
		checked, _ := p.authorization.AbilityOf(r, file, name)
		b.WriteString("```php\n(ability) " + name.Name + "\n```")
		for _, ability := range abilities {
			source := abilitySource(ability)
			if ability == checked && len(abilities) > 1 {
				source = "**" + source + "**"
			}
			b.WriteString("\n\n" + source)
		}
		return markdownHover(b.String())
	}
	registered := p.authorization.Middleware(name.Name)
	if len(registered) == 0 {
		return nil
	}
	m := registered[0]
	if m.Kind == frameworklaravel.MiddlewareGroup {
		b.WriteString("```php\n(middleware group) " + name.Name + "\n```")
	} else {
		b.WriteString("```php\n(middleware) " + name.Name + "\n```")
		if m.Class != "" {
			b.WriteString("\n\n`" + m.Class + "`")
		}
	}
	b.WriteString("\n\nRegistered in `" + path.Base(m.URI) + "`")
	return markdownHover(b.String())
}

func abilitySource(ability frameworklaravel.Ability) string {
	if ability.Policy != "" {
		return "`" + ability.Policy + "::" + ability.Method + "()`"
	}
	return "`Gate::define()` in `" + path.Base(ability.URI) + "`"
}

func markdownHover(value string) *protocol.Hover {
	return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: value}}
}
