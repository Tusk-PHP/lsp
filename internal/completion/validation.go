package completion

import (
	"strconv"
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// SetValidationRules sets the Laravel validation rules completed in rule
// strings.
func (p *Provider) SetValidationRules(rules *frameworklaravel.ValidationRules) {
	p.validation = rules
}

func (p *Provider) completeValidationRules(source string, pos protocol.Position, file *parser.FileNode) ([]protocol.CompletionItem, bool) {
	if p.framework != "laravel" || p.validation == nil {
		return nil, false
	}
	syn := p.validation.SyntaxOf(p.engine.Resolver(), source, file)
	slot, ok := frameworklaravel.ValidationRuleSlotAt(source, pos, syn)
	if !ok {
		return nil, false
	}
	if !slot.InName {
		return []protocol.CompletionItem{}, true
	}
	typed := frameworklaravel.ValidationRuleKey(slot.Typed)
	rules := p.validation.All()
	items := make([]protocol.CompletionItem, 0, len(rules))
	for _, rule := range rules {
		if !strings.HasPrefix(frameworklaravel.ValidationRuleKey(rule.Name), typed) {
			continue
		}
		item := protocol.CompletionItem{
			Label:         rule.Name,
			Kind:          protocol.CompletionItemKindValue,
			Detail:        rule.Signature(),
			Documentation: rule.Description(),
			FilterText:    rule.Name,
			SortText:      "0" + rule.Name,
			TextEdit:      &protocol.TextEdit{Range: slot.TypedRange, NewText: rule.Name},
		}
		if rule.Required > 0 && !slot.HasParameters {
			item.TextEdit.NewText = validationRuleSnippet(rule)
			item.InsertTextFormat = 2
		}
		items = append(items, item)
	}
	return items, true
}

func validationRuleSnippet(rule frameworklaravel.ValidationRule) string {
	var b strings.Builder
	b.WriteString(rule.Name)
	b.WriteByte(':')
	for i := 0; i < rule.Required; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("${")
		b.WriteString(strconv.Itoa(i + 1))
		if i < len(rule.Parameters) && rule.Parameters[i] != "…" {
			b.WriteByte(':')
			b.WriteString(rule.Parameters[i])
		}
		b.WriteByte('}')
	}
	return b.String()
}
