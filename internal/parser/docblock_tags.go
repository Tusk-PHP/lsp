package parser

import (
	"iter"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/types"
)

// docTag is one @tag line of a docblock, before it's interpreted.
type docTag struct {
	name  string
	value string
}

// splitDocTag splits "return Foo $x" into its name and value.
func splitDocTag(s string) (name, value string) {
	end := strings.IndexAny(s, " \t")
	if end < 0 {
		return s, ""
	}
	return s[:end], strings.TrimSpace(s[end+1:])
}

func docLines(comment string) iter.Seq[string] {
	return func(yield func(string) bool) {
		body := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(comment), "/*"), "*/")
		for line := range strings.Lines(body) {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
			if line != "" && !yield(line) {
				return
			}
		}
	}
}

// docTagLayout says where the value of a PHPDoc tag holds types.
type docTagLayout uint8

const (
	docLayoutProse docTagLayout = iota
	docLayoutVariableType
	docLayoutLeadingType
	docLayoutAssert
	docLayoutTemplate
	docLayoutTypeAlias
	docLayoutImportType
	docLayoutMethod
	docLayoutReference
)

// docTagRead says in which forms ParseDocBlock interprets a tag: a bare
// @assert is PHPUnit's own annotation and a bare @type is WordPress's hash
// notation, so those are only read with a phpstan- or psalm- prefix.
type docTagRead uint8

const (
	docReadNever docTagRead = iota
	docReadAlways
	docReadPlain
	docReadPrefixed
)

type docTagSpec struct {
	layout docTagLayout
	read   docTagRead
}

// docTagSpecOf is the one list of the PHPDoc tags this package knows, keyed by
// the base name docTagName gives: where each holds types, and whether
// ParseDocBlock interprets it.
func docTagSpecOf(base string) docTagSpec {
	switch base {
	case "param", "var", "param-out", "param-closure-this":
		return docTagSpec{docLayoutVariableType, docReadAlways}
	case "property", "property-read", "property-write":
		return docTagSpec{docLayoutVariableType, docReadPlain}
	case "global":
		return docTagSpec{docLayoutVariableType, docReadNever}
	case "return", "extends", "implements", "use", "self-out", "mixin":
		return docTagSpec{docLayoutLeadingType, docReadAlways}
	case "throws":
		return docTagSpec{docLayoutLeadingType, docReadPlain}
	case "require-extends", "require-implements", "sealed", "inheritors", "if-this-is", "scope-this", "yield", "expectedException":
		return docTagSpec{docLayoutLeadingType, docReadNever}
	case "assert", "assert-if-true", "assert-if-false":
		return docTagSpec{docLayoutAssert, docReadPrefixed}
	case "template":
		return docTagSpec{docLayoutTemplate, docReadAlways}
	case "type":
		return docTagSpec{docLayoutTypeAlias, docReadPrefixed}
	case "import-type":
		return docTagSpec{docLayoutImportType, docReadPrefixed}
	case "method":
		return docTagSpec{docLayoutMethod, docReadPlain}
	case "deprecated":
		return docTagSpec{docLayoutProse, docReadPlain}
	case "see", "link", "covers", "coversDefaultClass", "uses":
		return docTagSpec{docLayoutReference, docReadNever}
	}
	return docTagSpec{}
}

// docTagBase maps a tag name to the tag it stands for and its precedence:
// @phpstan-X beats @psalm-X, which beats @X. ok is false for tags this parser
// doesn't interpret.
func docTagBase(name string) (base string, prio int, ok bool) {
	base, prio = docTagName(name)
	switch docTagSpecOf(base).read {
	case docReadAlways:
		return base, prio, true
	case docReadPlain:
		return base, prio, prio == 0
	case docReadPrefixed:
		return base, prio, prio > 0
	}
	return "", 0, false
}

func docTagName(name string) (base string, prio int) {
	switch {
	case strings.HasPrefix(name, "phpstan-"):
		base, prio = name[len("phpstan-"):], 2
	case strings.HasPrefix(name, "psalm-"):
		base, prio = name[len("psalm-"):], 1
	default:
		base = name
	}
	switch base {
	case "template-extends":
		base = "extends"
	case "template-implements":
		base = "implements"
	case "template-use":
		base = "use"
	case "this-out":
		base = "self-out"
	case "template-covariant", "template-contravariant":
		base = "template"
	}
	return base, prio
}

type pickedTag struct {
	base, key, value string
	prio, order      int
}

// applyDocTags interprets the docblock's tags. Where a prefixed and a plain tag
// describe the same thing (the same parameter, the return type, the same
// parent class), only the highest-precedence one is kept.
func applyDocTags(doc *DocBlock, tags []docTag) {
	picked := make([]pickedTag, 0, len(tags))
	slot := make(map[string]int, len(tags))
	for i, t := range tags {
		base, prio, ok := docTagBase(t.name)
		if !ok {
			continue
		}
		key := base + "|" + docTagKey(base, t.value)
		// Lists such as several @throws or @method tags keep every entry.
		if !docTagRepeats(base) {
			if j, seen := slot[key]; seen {
				if prio > picked[j].prio {
					picked[j].value, picked[j].prio = t.value, prio
				}
				continue
			}
			slot[key] = len(picked)
		}
		picked = append(picked, pickedTag{base: base, key: key, value: t.value, prio: prio, order: i})
	}

	var params, returns, vars []string
	for _, t := range picked {
		v := t.value
		switch t.base {
		case "param":
			doc.Params = append(doc.Params, parseDocParam(v))
			params = append(params, v)
		case "return":
			doc.Return = parseDocReturn(v)
			returns = append(returns, v)
		case "var":
			doc.Vars = append(doc.Vars, parseDocParam(v))
			vars = append(vars, v)
		case "throws":
			doc.Throws = append(doc.Throws, parseDocThrow(v))
		case "deprecated":
			doc.Deprecated = true
			doc.DeprecatedMsg = v
		case "property":
			doc.Properties = append(doc.Properties, parseDocProperty(v, false, false))
		case "property-read":
			doc.Properties = append(doc.Properties, parseDocProperty(v, true, false))
		case "property-write":
			doc.Properties = append(doc.Properties, parseDocProperty(v, false, true))
		case "method":
			doc.Methods = append(doc.Methods, parseDocMethod(v))
		case "template":
			if tmpl := parseDocTemplate(v); tmpl.Name != "" {
				doc.Templates = append(doc.Templates, tmpl)
			}
		case "type":
			if alias := parseDocTypeAlias(v); alias.Name != "" {
				doc.TypeAliases = append(doc.TypeAliases, alias)
			}
		case "import-type":
			if alias := parseDocImportedTypeAlias(v); alias.Name != "" && alias.From != "" {
				doc.ImportedTypes = append(doc.ImportedTypes, alias)
			}
		case "extends":
			if typ, _ := types.ExtractDocTypeString(v); typ != "" {
				doc.Extends = append(doc.Extends, typ)
			}
		case "implements":
			if typ, _ := types.ExtractDocTypeString(v); typ != "" {
				doc.Implements = append(doc.Implements, typ)
			}
		case "use":
			if typ, _ := types.ExtractDocTypeString(v); typ != "" {
				doc.Uses = append(doc.Uses, typ)
			}
		case "mixin":
			if typ, _ := types.ExtractDocTypeString(v); typ != "" {
				doc.Mixins = append(doc.Mixins, typ)
			}
		case "param-out":
			doc.ParamOuts = append(doc.ParamOuts, parseDocParam(v))
		case "param-closure-this":
			doc.ClosureThis = append(doc.ClosureThis, parseDocParam(v))
		case "self-out":
			doc.SelfOut, _ = types.ExtractDocTypeString(v)
		case "assert":
			doc.Asserts = append(doc.Asserts, parseDocAssert(v, AssertAlways))
		case "assert-if-true":
			doc.Asserts = append(doc.Asserts, parseDocAssert(v, AssertIfTrue))
		case "assert-if-false":
			doc.Asserts = append(doc.Asserts, parseDocAssert(v, AssertIfFalse))
		}
	}

	// Readers of the raw tags see what the prefixed tags said, too.
	if params != nil {
		doc.Tags["param"] = params
	}
	if returns != nil {
		doc.Tags["return"] = returns
	}
	if vars != nil {
		doc.Tags["var"] = vars
	}
}

// docTagKey is what two tags must share to describe the same thing: the
// parameter name for @param, the class for @extends, nothing for @return.
func docTagKey(base, value string) string {
	switch base {
	case "param", "param-out", "param-closure-this", "var":
		_, rest := types.ExtractDocTypeString(value)
		if strings.HasPrefix(value, "$") {
			rest = value
		}
		if f := strings.Fields(rest); len(f) > 0 && strings.HasPrefix(f[0], "$") {
			return strings.TrimPrefix(f[0], "&")
		}
		return ""
	case "template":
		if f := strings.Fields(value); len(f) > 0 {
			return f[0]
		}
	case "extends", "implements", "use":
		typ, _ := types.ExtractDocTypeString(value)
		if i := strings.IndexByte(typ, '<'); i >= 0 {
			typ = typ[:i]
		}
		return strings.TrimPrefix(typ, "\\")
	case "return", "self-out":
		return ""
	case "assert", "assert-if-true", "assert-if-false":
		_, rest := types.ExtractDocTypeString(strings.TrimLeft(value, "!="))
		if f := strings.Fields(rest); len(f) > 0 {
			return f[0]
		}
	}
	// Everything else is a list where each tag stands alone.
	return value
}

// docTagRepeats reports whether a tag may appear several times for the same
// key and keep every occurrence.
func docTagRepeats(base string) bool {
	switch base {
	case "throws", "property", "property-read", "property-write", "method", "mixin", "deprecated":
		return true
	}
	return false
}

// parseDocTemplate parses "@template TModel", "@template TModel of Model" and
// "@template T of array<int, Foo> = array<int, Foo>".
func parseDocTemplate(value string) DocTemplate {
	t := DocTemplate{}
	value = strings.TrimSpace(value)
	end := strings.IndexAny(value, " \t")
	if end < 0 {
		t.Name = value
		return t
	}
	t.Name = value[:end]
	rest := strings.TrimSpace(value[end:])
	if strings.HasPrefix(rest, "of ") || strings.HasPrefix(rest, "as ") {
		t.Bound, rest = types.ExtractDocTypeString(rest[3:])
		t.ParsedBound = parseDocTypeExprPtr(t.Bound)
	}
	if strings.HasPrefix(rest, "=") {
		t.Default, _ = types.ExtractDocTypeString(strings.TrimSpace(rest[1:]))
	}
	return t
}

// parseDocAssert parses "!Foo $value", "=int $this->count" and the like.
func parseDocAssert(value string, kind AssertKind) DocAssert {
	a := DocAssert{Kind: kind}
	value = strings.TrimSpace(value)
	for len(value) > 0 && (value[0] == '!' || value[0] == '=') {
		if value[0] == '!' {
			a.Negated = true
		} else {
			a.Equality = true
		}
		value = strings.TrimSpace(value[1:])
	}
	var rest string
	a.Type, rest = types.ExtractDocTypeString(value)
	if f := strings.Fields(rest); len(f) > 0 {
		a.Target = f[0]
	}
	return a
}
