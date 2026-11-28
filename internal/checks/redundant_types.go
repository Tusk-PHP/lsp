package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// RedundantNullsafeRule reports `?->` on a receiver that can't be null.
// It only reports what it can be sure of without following types: `$this`,
// and a parameter declared with a type that excludes null, with no `null`
// default, that its function only ever uses as a receiver, so nothing
// reassigns it. Anything else stays quiet: the types the resolver gives
// lose nullability (a `?Foo $x = null` parameter, a ternary with `null`).
type RedundantNullsafeRule struct {
	// TypeResolver is kept for the rule set's wiring; the rule no longer
	// relies on it. If nil, the rule is a no-op.
	TypeResolver func(expr string, source string, line, col int, file *parser.FileNode) string
}

func (r *RedundantNullsafeRule) Codes() []rule.Code { return []rule.Code{rule.RedundantNullsafe} }

func (r *RedundantNullsafeRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if r.TypeResolver == nil || file == nil || !strings.Contains(source, "?->") {
		return nil
	}
	toks := significantTokens(parser.Tokenize(source))
	fns := functionSpans(file)
	var findings []Finding
	for i := 1; i+1 < len(toks); i++ {
		q := toks[i]
		if q.Kind != parser.TokenQuestion || toks[i+1].Kind != parser.TokenArrow || toks[i+1].Offset != q.Offset+1 {
			continue
		}
		recv := toks[i-1]
		if recv.Kind != parser.TokenVariable {
			continue
		}
		// `$a->b?->` and `A::$b?->` are properties, not the variable.
		if i >= 2 && (toks[i-2].Kind == parser.TokenArrow || toks[i-2].Kind == parser.TokenDoubleColon) {
			continue
		}
		message := ""
		if recv.Value == "$this" {
			message = "Redundant nullsafe operator: $this is never null"
		} else if typ, ok := nonNullParameter(toks, fns, recv); ok {
			message = fmt.Sprintf("Redundant nullsafe operator: type '%s' is non-nullable", typ)
		} else {
			continue
		}
		findings = append(findings, Finding{
			StartLine: q.Line,
			StartCol:  q.Column,
			EndLine:   q.Line,
			EndCol:    q.Column + 3,
			Severity:  rule.SeverityInfo,
			Code:      rule.RedundantNullsafe,
			Message:   message,
		})
	}
	return findings
}

// significantTokens drops comments.
func significantTokens(all []parser.Token) []parser.Token {
	out := make([]parser.Token, 0, len(all))
	for _, t := range all {
		if t.Kind != parser.TokenComment && t.Kind != parser.TokenDocComment && t.Kind != parser.TokenEOF {
			out = append(out, t)
		}
	}
	return out
}

type functionSpan struct {
	start, end int
	params     []parser.ParamNode
}

// functionSpans lists the file's functions and methods by line.
func functionSpans(file *parser.FileNode) []functionSpan {
	var out []functionSpan
	addMethods := func(methods []parser.MethodNode) {
		for _, m := range methods {
			out = append(out, functionSpan{m.StartLine, m.EndLine, m.Params})
		}
	}
	for _, c := range file.Classes {
		addMethods(c.Methods)
	}
	for _, c := range file.AnonymousClasses {
		addMethods(c.Methods)
	}
	for _, t := range file.Traits {
		addMethods(t.Methods)
	}
	for _, e := range file.Enums {
		addMethods(e.Methods)
	}
	for _, f := range file.Functions {
		out = append(out, functionSpan{f.StartLine, f.EndLine, f.Params})
	}
	return out
}

// nonNullParameter reports whether recv is a parameter of the innermost
// function around it whose declared type excludes null, with no null
// default, and which the function only uses as a receiver (`$x->`, `$x?->`)
// after declaring it, so it is never reassigned or passed by reference.
func nonNullParameter(toks []parser.Token, fns []functionSpan, recv parser.Token) (string, bool) {
	var fn *functionSpan
	for i := range fns {
		f := &fns[i]
		if f.start <= recv.Line && recv.Line <= f.end && (fn == nil || f.end-f.start < fn.end-fn.start) {
			fn = f
		}
	}
	if fn == nil {
		return "", false
	}
	var param *parser.ParamNode
	for i := range fn.params {
		if fn.params[i].Name == recv.Value {
			param = &fn.params[i]
		}
	}
	if param == nil || param.IsVariadic || param.IsReference || strings.EqualFold(strings.TrimSpace(param.DefaultValue), "null") {
		return "", false
	}
	typ := strings.TrimSpace(param.Type.Name)
	if typ == "" || isNullable(typ) || strings.Contains(strings.ToLower(typ), "mixed") {
		return "", false
	}
	declared := false
	for i, t := range toks {
		if t.Line < fn.start || t.Line > fn.end || t.Kind != parser.TokenVariable || t.Value != recv.Value {
			continue
		}
		if !declared {
			declared = true // the parameter itself, in the signature
			continue
		}
		next := parser.Token{Kind: parser.TokenEOF}
		if i+1 < len(toks) {
			next = toks[i+1]
		}
		if next.Kind == parser.TokenArrow {
			continue
		}
		if next.Kind == parser.TokenQuestion && i+2 < len(toks) && toks[i+2].Kind == parser.TokenArrow {
			continue
		}
		return "", false
	}
	return typ, declared
}

// isNullable returns true if the type can be null.
func isNullable(typ string) bool {
	if typ == "" || typ == "mixed" || typ == "null" {
		return true
	}
	expr := types.ParseTypeExpr(typ)
	if expr.Nullable {
		return true
	}
	for _, member := range expr.UnionMembers() {
		if member.IsNull() {
			return true
		}
	}
	return false
}

// RedundantUnionRule detects union type declarations with duplicate members
// or types that are supertypes of other members.
type RedundantUnionRule struct{}

func (r *RedundantUnionRule) Codes() []rule.Code { return []rule.Code{rule.RedundantUnionMember} }

func (r *RedundantUnionRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	var findings []Finding

	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			// Check parameter types
			for _, p := range m.Params {
				findings = append(findings, checkUnionType(p.Type.Name, m.StartLine, index)...)
			}
			// Check return type
			findings = append(findings, checkUnionType(m.ReturnType.Name, m.StartLine, index)...)
		}
		for _, p := range cls.Properties {
			findings = append(findings, checkUnionType(p.Type.Name, p.StartLine, index)...)
		}
	}
	for _, fn := range file.Functions {
		for _, p := range fn.Params {
			findings = append(findings, checkUnionType(p.Type.Name, fn.StartLine, index)...)
		}
		findings = append(findings, checkUnionType(fn.ReturnType.Name, fn.StartLine, index)...)
	}
	return findings
}

// supertypes maps types that subsume other types.
var supertypes = map[string][]string{
	"mixed":    {"string", "int", "float", "bool", "array", "object", "null", "callable", "iterable", "void", "never"},
	"object":   {}, // any class name is subsumed
	"iterable": {"array"},
}

func writtenMember(member types.TypeExpr) string {
	if member.Kind == types.TypeKindNamed {
		return member.Name
	}
	return member.String()
}

func checkUnionType(typeStr string, line int, index *symbols.Index) []Finding {
	if typeStr == "" {
		return nil
	}

	expr := types.ParseTypeExpr(typeStr)
	members := expr.UnionMembers()
	parts := make([]string, 0, len(members)+1)
	for _, member := range members {
		parts = append(parts, writtenMember(member))
	}
	if expr.Nullable {
		parts = append(parts, "null")
	}
	if len(parts) < 2 {
		return nil
	}

	var findings []Finding

	// Check for duplicates
	seen := make(map[string]bool)
	for _, p := range parts {
		lower := strings.ToLower(p)
		if seen[lower] {
			findings = append(findings, Finding{
				StartLine: line,
				StartCol:  0,
				EndLine:   line,
				EndCol:    0,
				Severity:  rule.SeverityInfo,
				Code:      rule.RedundantUnionMember,
				Message:   fmt.Sprintf("Duplicate type '%s' in union", p),
			})
		}
		seen[lower] = true
	}

	// Check ?Type|null redundancy (the ? already makes it nullable)
	if expr.Nullable {
		for _, member := range members {
			if member.IsNull() {
				findings = append(findings, Finding{
					StartLine: line,
					StartCol:  0,
					EndLine:   line,
					EndCol:    0,
					Severity:  rule.SeverityInfo,
					Code:      rule.RedundantUnionMember,
					Message:   fmt.Sprintf("Redundant 'null' in union: '?' already makes '%s' nullable", typeStr),
				})
				break
			}
		}
	}

	// Check for supertype subsumption
	partSet := make(map[string]bool)
	for _, p := range parts {
		partSet[strings.ToLower(p)] = true
	}

	for _, p := range parts {
		lower := strings.ToLower(p)

		// Check if any supertype is present
		for super, subsumed := range supertypes {
			if !partSet[super] || super == lower {
				continue
			}
			// "mixed" subsumes everything
			if super == "mixed" {
				findings = append(findings, Finding{
					StartLine: line,
					StartCol:  0,
					EndLine:   line,
					EndCol:    0,
					Severity:  rule.SeverityInfo,
					Code:      rule.RedundantUnionMember,
					Message:   fmt.Sprintf("Redundant type '%s' in union: already covered by 'mixed'", p),
				})
				break
			}
			// Check if p is in the known subsumed list
			for _, s := range subsumed {
				if lower == s {
					findings = append(findings, Finding{
						StartLine: line,
						StartCol:  0,
						EndLine:   line,
						EndCol:    0,
						Severity:  rule.SeverityInfo,
						Code:      rule.RedundantUnionMember,
						Message:   fmt.Sprintf("Redundant type '%s' in union: already covered by '%s'", p, super),
					})
					break
				}
			}
			// "object" subsumes any class name
			if super == "object" && !symbols.IsPHPBuiltinType(lower) && lower != "null" {
				findings = append(findings, Finding{
					StartLine: line,
					StartCol:  0,
					EndLine:   line,
					EndCol:    0,
					Severity:  rule.SeverityInfo,
					Code:      rule.RedundantUnionMember,
					Message:   fmt.Sprintf("Redundant type '%s' in union: already covered by 'object'", p),
				})
			}
		}

		// Check inheritance: if both parent and child class in union
		if index != nil && !symbols.IsPHPBuiltinType(lower) && lower != "null" {
			chain := index.GetInheritanceChain(p)
			for _, parent := range chain {
				if partSet[strings.ToLower(parent)] {
					findings = append(findings, Finding{
						StartLine: line,
						StartCol:  0,
						EndLine:   line,
						EndCol:    0,
						Severity:  rule.SeverityInfo,
						Code:      rule.RedundantUnionMember,
						Message:   fmt.Sprintf("Redundant type '%s' in union: already covered by parent '%s'", p, parent),
					})
					break
				}
			}
		}
	}

	return findings
}
