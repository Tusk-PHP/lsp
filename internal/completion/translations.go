package completion

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// completeLaravelTranslations offers the Laravel translation keys that match
// what is typed, with the locale and file they come from.
func (p *Provider) completeLaravelTranslations(ctx *frameworklaravel.TranslationStringContext) []protocol.CompletionItem {
	matches := p.translations.Matching(ctx)
	if matches == nil {
		return nil
	}
	items := make([]protocol.CompletionItem, 0, len(matches))
	for _, entry := range matches {
		items = append(items, translationItem(entry.Key, ctx.Partial, entry.Locale+" - "+entry.File))
	}
	return items
}

// completeSymfonyTranslations offers the Symfony translation keys that match
// what is typed, with their domain, locale and file.
func (p *Provider) completeSymfonyTranslations(ctx *frameworksymfony.TranslationContext) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, entry := range p.symfonyTranslations.Matching(ctx) {
		items = append(items, translationItem(entry.Key, ctx.Partial, entry.Domain+" ("+entry.Locale+") - "+entry.File))
	}
	return items
}

// translationItem offers a translation key. partial is what is typed of the
// key so far: when the key starts with it, only the rest is inserted.
func translationItem(key, partial, detail string) protocol.CompletionItem {
	insertText := key
	if strings.HasPrefix(key, partial) {
		insertText = key[len(partial):]
	}
	return protocol.CompletionItem{
		Label:      key,
		Kind:       protocol.CompletionItemKindValue,
		Detail:     detail,
		InsertText: insertText,
		FilterText: key,
		SortText:   "0" + key,
	}
}
