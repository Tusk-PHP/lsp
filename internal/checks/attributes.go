package checks

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// DeprecatedUsageRule flags uses of what's marked with PHP 8.4's #[\Deprecated].
type DeprecatedUsageRule struct {
	TypeResolver TypeResolver
	// Uses, when set, shares the attribute uses found in a file with the
	// other rules run over it.
	Uses *MemberUseCache
}

func (r *DeprecatedUsageRule) Codes() []rule.Code { return []rule.Code{rule.Deprecated} }

func (r *DeprecatedUsageRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	var findings []Finding
	for _, use := range r.Uses.get(file, source, index, r.TypeResolver) {
		attr := use.symbol.Attribute("Deprecated")
		if attr == nil {
			continue
		}
		message := fmt.Sprintf("'%s' is deprecated", use.symbol.Name)
		if reason := attributeMessage(attr); reason != "" {
			message += ": " + reason
		}
		findings = append(findings, use.finding(rule.Deprecated, message, []Tag{TagDeprecated}))
	}
	return findings
}

// UnusedResultRule flags calls to what's marked with PHP 8.5's #[\NoDiscard]
// whose result is thrown away. A `(void)` cast is the way to do that on purpose.
type UnusedResultRule struct {
	TypeResolver TypeResolver
	// Uses, when set, shares the attribute uses found in a file with the
	// other rules run over it.
	Uses *MemberUseCache
}

func (r *UnusedResultRule) Codes() []rule.Code { return []rule.Code{rule.UnusedResult} }

func (r *UnusedResultRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	var findings []Finding
	for _, use := range r.Uses.get(file, source, index, r.TypeResolver) {
		attr := use.symbol.Attribute("NoDiscard")
		if attr == nil || !use.isCall || !use.isStatement {
			continue
		}
		message := fmt.Sprintf("The result of '%s' should be used", use.symbol.Name)
		if reason := attributeMessage(attr); reason != "" {
			message += ": " + reason
		}
		findings = append(findings, use.finding(rule.UnusedResult, message, nil))
	}
	return findings
}

// InvalidOverrideRule flags methods with PHP 8.3's #[\Override], and class
// constants and enum cases with PHP 8.6's, that override nothing, which PHP
// refuses to compile. It stays quiet when an ancestor isn't indexed, since
// the member could be overriding something there.
type InvalidOverrideRule struct{}

func (r *InvalidOverrideRule) Codes() []rule.Code { return []rule.Code{rule.InvalidOverride} }

func (r *InvalidOverrideRule) Check(file *parser.FileNode, _ string, index *symbols.Index) []Finding {
	if file == nil || index == nil {
		return nil
	}
	var findings []Finding
	check := func(name string, methods []parser.MethodNode) {
		fqn := name
		if file.Namespace != "" {
			fqn = file.Namespace + "\\" + name
		}
		for _, m := range methods {
			if !hasOverrideAttribute(m.Attributes, file) || strings.EqualFold(m.Name, "__toString") {
				continue
			}
			if overrides, known := index.OverridesInherited(fqn, m.Name); known && !overrides {
				findings = append(findings, Finding{
					StartLine: m.StartLine,
					StartCol:  m.StartCol,
					EndLine:   m.StartLine,
					EndCol:    m.StartCol + len(m.Name),
					Severity:  rule.SeverityError,
					Code:      rule.InvalidOverride,
					Message:   fmt.Sprintf("'%s' has #[\\Override], but there's no such method to override", m.Name),
				})
			}
		}
	}
	// PHP 8.6 allows #[\Override] on class constants and enum cases too.
	checkConstant := func(class, name string, attrs []parser.AttributeDef, line, col int) {
		if !hasOverrideAttribute(attrs, file) {
			return
		}
		fqn := class
		if file.Namespace != "" {
			fqn = file.Namespace + "\\" + class
		}
		if overrides, known := index.OverridesInheritedConstant(fqn, name); known && !overrides {
			findings = append(findings, Finding{
				StartLine: line,
				StartCol:  col,
				EndLine:   line,
				EndCol:    col + len(name),
				Severity:  rule.SeverityError,
				Code:      rule.InvalidOverride,
				Message:   fmt.Sprintf("'%s' has #[\\Override], but there's no such constant to override", name),
			})
		}
	}
	for _, c := range file.Classes {
		check(c.Name, c.Methods)
		for _, k := range c.Constants {
			checkConstant(c.Name, k.Name, k.Attributes, k.StartLine, k.StartCol)
		}
	}
	for _, e := range file.Enums {
		check(e.Name, e.Methods)
		for _, k := range e.Constants {
			checkConstant(e.Name, k.Name, k.Attributes, k.StartLine, k.StartCol)
		}
		for _, k := range e.Cases {
			checkConstant(e.Name, k.Name, k.Attributes, k.StartLine, k.StartCol)
		}
	}
	return findings
}

func hasOverrideAttribute(attrs []parser.AttributeDef, file *parser.FileNode) bool {
	for _, attr := range attrs {
		if attr.Name == "\\Override" || (attr.Name == "Override" && file.Namespace == "") {
			return true
		}
		for _, use := range file.Uses {
			if use.FullName == "Override" && strings.EqualFold(use.Alias, attr.Name) {
				return true
			}
		}
	}
	return false
}

// attributeMessage returns the `message` argument of #[\Deprecated] or
// #[\NoDiscard], which is the first one unless it's passed by name.
func attributeMessage(attr *symbols.Attribute) string {
	message, _ := attr.Args.Get("message", 0)
	return strings.Trim(message, `'"`)
}

// memberUse is a function, method or constant used in the source.
type memberUse struct {
	symbol      *symbols.Symbol
	site        Finding
	isCall      bool
	isStatement bool // nothing takes its result
}

// MemberUseCache holds the attribute uses found in one file, so the rules
// that read them (deprecated, unused-result) find them once. A nil cache
// finds them every time.
type MemberUseCache struct {
	// Shared, when set, shares the file's syntax tree with the other rules.
	Shared *Shared
	source string
	file   *parser.FileNode
	uses   []memberUse
	done   bool
}

func (c *MemberUseCache) get(file *parser.FileNode, source string, index *symbols.Index, typeResolver TypeResolver) []memberUse {
	if c == nil {
		return memberUses(file, source, index, typeResolver, nil)
	}
	if !c.done || c.source != source || c.file != file {
		c.source, c.file, c.done = source, file, true
		c.uses = memberUses(file, source, index, typeResolver, c.Shared)
	}
	return c.uses
}

func (u memberUse) finding(code rule.Code, message string, tags []Tag) Finding {
	f := u.site
	f.Severity, f.Code, f.Message, f.Tags = rule.SeverityWarning, code, message, tags
	return f
}

func memberUses(file *parser.FileNode, source string, index *symbols.Index, typeResolver TypeResolver, shared *Shared) []memberUse {
	if file == nil || index == nil {
		return nil
	}
	// Most files use no attributes at all, so don't resolve anything for them.
	if !index.HasAttributedSymbols() {
		return nil
	}
	syntax := shared.tree(source, file)
	refs := syntax.references()
	var uses []memberUse
	add := func(sym *symbols.Symbol, name ast.Node, isCall, isStatement bool) {
		if sym != nil && len(sym.Attributes) > 0 {
			uses = append(uses, memberUse{symbol: sym, site: syntax.at(name, Finding{}), isCall: isCall, isStatement: isCall && isStatement})
		}
	}
	for _, call := range refs.calls {
		add(lookupCalledFunction(syntax.text(call.name), file, index), call.name, true, call.statement)
	}
	for _, ref := range refs.members {
		if !ref.static && !ref.call() {
			continue
		}
		recv, ok := syntax.receiverOf(ref)
		if !ok {
			continue
		}
		owners := recv.classLikes(file, index, typeResolver)
		if len(owners) == 0 {
			continue
		}
		add(classMember(index, owners[0], syntax.text(ref.name), ref.call()), ref.name, ref.call(), ref.statement)
	}
	slices.SortFunc(uses, func(a, b memberUse) int {
		return cmp.Or(cmp.Compare(a.site.StartLine, b.site.StartLine), cmp.Compare(a.site.StartCol, b.site.StartCol))
	})
	return uses
}

func classMember(index *symbols.Index, class, name string, isCall bool) *symbols.Symbol {
	for _, member := range index.GetClassMembers(class) {
		if member.Name == name && member.Kind != symbols.KindProperty && (member.Kind == symbols.KindMethod) == isCall {
			return member
		}
	}
	return nil
}

func lookupCalledFunction(name string, file *parser.FileNode, index *symbols.Index) *symbols.Symbol {
	fqns, _ := resolve.FunctionNames(name, file)
	for _, fqn := range fqns {
		if sym := index.LookupFold(fqn, isFunctionSymbol); sym != nil {
			return sym
		}
	}
	return nil
}
