package hover

import (
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// SetValidationRules sets the Laravel validation rules hovered in rule
// strings.
func (p *Provider) SetValidationRules(rules *frameworklaravel.ValidationRules) {
	p.validation = rules
}

func (p *Provider) hoverValidationRule(source string, pos protocol.Position, file *parser.FileNode) *protocol.Hover {
	if p.framework != "laravel" || p.validation == nil {
		return nil
	}
	syn := p.validation.SyntaxOf(p.engine.Resolver(), source, file)
	slot, ok := frameworklaravel.ValidationRuleSlotAt(source, pos, syn)
	if !ok {
		return nil
	}
	rule, ok := p.validation.Lookup(slot.Name)
	if !ok {
		return nil
	}
	value := "```php\n(validation rule) " + rule.Signature() + "\n```"
	if d := rule.Description(); d != "" {
		value += "\n\n" + d
	}
	r := slot.NameRange
	return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: value}, Range: &r}
}
