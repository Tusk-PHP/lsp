package completion

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/types"
)

type configResultArrayContext struct {
	ConfigArg  string
	AccessKeys []string
	Partial    string
	Quote      string
}

const configHelper = "config"

func configResultContext(ctx *sourcectx.CursorContext) *configResultArrayContext {
	dim, ok := ctx.StringDim()
	if !ok {
		return nil
	}
	src := ctx.File.Source
	var keys []string
	e := dim.Fetch.Array
	for fetch, ok := e.(*ast.ArrayDimFetch); ok; fetch, ok = e.(*ast.ArrayDimFetch) {
		key, isString := ast.StringValue(src, fetch.Dim)
		if !isString {
			return nil
		}
		keys = append(keys, key)
		e = fetch.Array
	}
	call, ok := e.(*ast.Call)
	if !ok || ast.FunctionName(src, call) != configHelper || len(call.Args.Args) == 0 {
		return nil
	}
	configArg, ok := ast.StringValue(src, call.Args.Args[0].Value)
	if !ok || configArg == "" {
		return nil
	}
	slices.Reverse(keys)
	return &configResultArrayContext{ConfigArg: configArg, AccessKeys: keys, Partial: dim.Partial, Quote: dim.Quote}
}

func (p *Provider) completeConfigResultKeys(ctx *configResultArrayContext) []protocol.CompletionItem {
	if p.arrayResolver == nil {
		return nil
	}

	parts := strings.Split(ctx.ConfigArg, ".")
	configFile := parts[0]
	keys := p.arrayResolver.ParseConfigFile(configFile)
	if keys == nil {
		return nil
	}

	for _, segment := range parts[1:] {
		var nestedType string
		for _, f := range keys {
			if f.Key == segment {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil
		}
		keys = types.ParseArrayShape(nestedType)
		if keys == nil {
			return nil
		}
	}

	for _, accessKey := range ctx.AccessKeys {
		var nestedType string
		for _, f := range keys {
			if f.Key == accessKey {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil
		}
		keys = types.ParseArrayShape(nestedType)
		if keys == nil {
			return nil
		}
	}

	if len(keys) == 0 {
		return nil
	}

	q := "'"
	if ctx.Quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(ctx.Partial)
	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}
		insertText := k.Key
		if ctx.Quote == "" {
			insertText = q + k.Key + q
		}
		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     k.Type,
			InsertText: insertText,
			SortText:   "0" + k.Key,
		})
	}
	return items
}

func configArgContext(arg ast.CallArg) (configPath, partial, quote string, ok bool) {
	if arg.Function != configHelper || arg.Index != 0 || arg.InArray {
		return "", "", "", false
	}
	lastDot := strings.LastIndex(arg.Partial, ".")
	if lastDot < 0 {
		return "", arg.Partial, arg.Quote, true
	}
	return arg.Partial[:lastDot], arg.Partial[lastDot+1:], arg.Quote, true
}

func (p *Provider) completeConfigKeys(configPath, partial, quote string) []protocol.CompletionItem {
	if p.arrayResolver == nil {
		return nil
	}

	var keys []types.ShapeField
	if configPath == "" {
		keys = p.arrayResolver.ListConfigFiles()
	} else {
		parts := strings.Split(configPath, ".")
		configFile := parts[0]
		keys = p.arrayResolver.ParseConfigFile(configFile)
		if keys == nil {
			return nil
		}
		for _, segment := range parts[1:] {
			var nestedType string
			for _, f := range keys {
				if f.Key == segment {
					nestedType = f.Type
					break
				}
			}
			if nestedType == "" {
				return nil
			}
			keys = types.ParseArrayShape(nestedType)
			if keys == nil {
				return nil
			}
		}
	}

	if len(keys) == 0 {
		return nil
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(partial)

	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}

		isNested := strings.HasPrefix(k.Type, "array{") || k.Type == "array"
		detail := k.Type
		if isNested && strings.HasPrefix(k.Type, "array{") {
			inner := types.ParseArrayShape(k.Type)
			if len(inner) > 0 {
				var names []string
				for _, f := range inner {
					if f.Key != "" {
						names = append(names, f.Key)
					}
					if len(names) >= 4 {
						names = append(names, "...")
						break
					}
				}
				detail = "{" + strings.Join(names, ", ") + "}"
			}
		}

		insertText := k.Key
		if isNested {
			insertText = k.Key + "."
		}
		if quote == "" {
			insertText = "'" + insertText + "'"
		}

		kind := protocol.CompletionItemKindProperty
		if isNested {
			kind = protocol.CompletionItemKindModule
		}

		sortText := "0" + k.Key
		if isNested {
			sortText = "1" + k.Key
		}

		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       kind,
			Detail:     detail,
			InsertText: insertText,
			SortText:   sortText,
		})
	}
	return items
}
