package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ReturnsCallingClass reports whether a method returns the class it was called
// on: `static`, `$this`, or `self` (possibly nullable). For `$child->copy()`
// declared in Base as `: self`, that's the object's own class at runtime, so
// callers treat all three alike and use the class the method was called on.
func (r *Resolver) ReturnsCallingClass(member *symbols.Symbol) bool {
	if member == nil || member.Kind != symbols.KindMethod {
		return false
	}
	t := strings.TrimPrefix(strings.TrimSpace(r.rawMemberReturnType(member)), "?")
	if strings.Contains(t, "|") {
		t = PickBestUnionPart(t)
	}
	switch strings.TrimPrefix(t, "?") {
	case "static", "self", "$this":
		return true
	}
	return false
}

// rawMemberReturnType returns a member's return type (or a property's type)
// as written, choosing between its native type and its PHPDoc type.
func (r *Resolver) rawMemberReturnType(member *symbols.Symbol) string {
	ownerFQN := ownerScopeFQN(member, protocol.Position{}, nil)
	switch member.Kind {
	case symbols.KindMethod:
		var docType string
		if member.DocComment != "" {
			if doc := r.parseDoc(member.DocComment); doc != nil {
				docType = doc.Return.Type
			}
		}
		if t := r.preferDocType(member.ReturnType, docType, member.Templates, ownerFQN); t != "" {
			return r.expandTypeAliases(t, ownerFQN, nil)
		}
	case symbols.KindProperty:
		var docType string
		if member.DocComment != "" && strings.Contains(member.DocComment, "var") {
			if doc := r.parseDoc(member.DocComment); doc != nil && len(doc.Vars) > 0 {
				docType = doc.Vars[0].Type
			}
		}
		if t := r.preferDocType(member.Type, docType, nil, ownerFQN); t != "" {
			return r.expandTypeAliases(t, ownerFQN, nil)
		}
	}
	return ""
}

// preferDocType picks between a member's native type and its PHPDoc type the
// way PHPStan does: the PHPDoc type is used when it says more, as a template, a
// generic, a shape, a callable signature, a narrower union or a subclass, and
// the native type is kept when the PHPDoc is vaguer or contradicts it.
func (r *Resolver) preferDocType(native, doc string, methodTemplates []symbols.TemplateParam, ownerFQN string) string {
	native, doc = strings.TrimSpace(native), strings.TrimSpace(doc)
	if doc == "" {
		return native
	}
	if native == "" {
		return doc
	}
	nativeBare := strings.TrimPrefix(strings.TrimPrefix(native, "?"), "\\")
	switch strings.ToLower(nativeBare) {
	case "object", "mixed", "array", "iterable", "callable", "closure", "traversable",
		"iterator", "iteratoraggregate", "generator", "static", "self", "$this", "string", "int", "bool", "float":
		if strings.EqualFold(nativeBare, "string") || strings.EqualFold(nativeBare, "int") || strings.EqualFold(nativeBare, "bool") || strings.EqualFold(nativeBare, "float") {
			// A scalar's PHPDoc is only read for what it can tell a reader of
			// classes: a conditional type or a template.
			if !strings.Contains(doc, " is ") && !r.mentionsTemplate(doc, methodTemplates, ownerFQN) {
				return native
			}
		}
		if strings.EqualFold(doc, "mixed") {
			return native
		}
		return doc
	}
	if strings.EqualFold(doc, "mixed") || strings.EqualFold(doc, "object") {
		return native
	}
	if r.mentionsTemplate(doc, methodTemplates, ownerFQN) || strings.Contains(doc, " is ") {
		return doc
	}
	docRT := r.applyEnv(r.parseType(doc), typeEnv{owner: ownerFQN})
	nativeRT := r.parseType(native)
	if docRT.IsEmpty() {
		return native
	}
	// Every class in the PHPDoc type must be the native class or below it.
	nativeClasses := nativeRT.ClassNames()
	for _, part := range docRT.Parts() {
		if part.FQN == "" || IsBuiltinTypeName(part.FQN) {
			if part.FQN == "" || nativeRT.FQN == part.FQN || len(nativeRT.Union) > 0 {
				continue
			}
			return native
		}
		ok := false
		for _, nc := range nativeClasses {
			if strings.EqualFold(nc, part.FQN) || r.Index != nil && r.Index.IsSubtypeOf(part.FQN, nc) {
				ok = true
				break
			}
		}
		if !ok {
			return native
		}
	}
	return doc
}

// mentionsTemplate reports whether a PHPDoc type names a template of the
// method or of the class that declares it.
func (r *Resolver) mentionsTemplate(doc string, methodTemplates []symbols.TemplateParam, ownerFQN string) bool {
	var classTemplates []symbols.TemplateParam
	if ownerFQN != "" && r.Index != nil {
		if sym := r.Index.Lookup(ownerFQN); sym != nil {
			classTemplates = sym.Templates
		}
	}
	if len(methodTemplates)+len(classTemplates) == 0 {
		return false
	}
	found := false
	r.parseType(doc).transform(func(t ResolvedType) ResolvedType {
		if found || len(t.Params) > 0 {
			return t
		}
		for _, list := range [][]symbols.TemplateParam{methodTemplates, classTemplates} {
			for _, tp := range list {
				if tp.Name == t.FQN {
					found = true
				}
			}
		}
		return t
	})
	return found
}
