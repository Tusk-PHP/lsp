package resolve

import "strings"

// isClosureExpr reports whether expr is a closure or arrow function.
func isClosureExpr(expr string) bool {
	e := expr
	if rest, ok := cutKeyword(e, "static"); ok {
		e = rest
	}
	for _, kw := range []string{"function", "fn"} {
		if rest, ok := cutKeyword(e, kw); ok && strings.HasPrefix(rest, "(") {
			return true
		}
		if strings.HasPrefix(e, kw+"(") {
			return true
		}
	}
	return false
}

// closureType types a closure or arrow function as Closure(params): return.
// Untyped parameters take their types from expected, the callable type the
// closure is passed for; the return type is the declared one or, without
// one, what the arrow function's body or the first return statement gives.
func (r *Resolver) closureType(expr string, expected *ResolvedType, ctx ExprContext) ResolvedType {
	e := expr
	if rest, ok := cutKeyword(e, "static"); ok {
		e = rest
	}
	arrow := false
	switch {
	case strings.HasPrefix(e, "fn"):
		arrow = true
		e = strings.TrimSpace(e[2:])
	case strings.HasPrefix(e, "function"):
		e = strings.TrimSpace(e[len("function"):])
	}
	e = strings.TrimPrefix(e, "&")
	if !strings.HasPrefix(e, "(") {
		return ResolvedType{FQN: "Closure"}
	}
	close := matchingParen(e, 0)
	if close < 0 {
		return ResolvedType{FQN: "Closure"}
	}
	paramList := e[1:close]
	rest := strings.TrimSpace(e[close+1:])
	if !arrow {
		if after, ok := cutKeyword(rest, "use"); ok && strings.HasPrefix(after, "(") {
			if uc := matchingParen(after, 0); uc >= 0 {
				rest = strings.TrimSpace(after[uc+1:])
			}
		}
	}
	declaredReturn := ""
	if strings.HasPrefix(rest, ":") {
		rest = strings.TrimSpace(rest[1:])
		end := len(rest)
		if arrow {
			if k := strings.Index(rest, "=>"); k >= 0 {
				end = k
			}
		} else if k := strings.IndexByte(rest, '{'); k >= 0 {
			end = k
		}
		declaredReturn = strings.TrimSpace(rest[:end])
		rest = strings.TrimSpace(rest[end:])
	}

	var expParams []ResolvedType
	if expected != nil {
		for _, p := range expected.Parts() {
			if p.Callable != nil {
				expParams = p.Callable.Params
				break
			}
		}
	}
	sig := &CallableType{}
	locals := make(map[string]ResolvedType, len(ctx.Locals)+4)
	for k, v := range ctx.Locals {
		locals[k] = v
	}
	for i, raw := range splitArgList(paramList) {
		name, declared := closureParamParts(raw)
		var pt ResolvedType
		if declared != "" {
			pt = r.resolveDocType(declared, ctx.File)
		} else if i < len(expParams) {
			pt = expParams[i]
		}
		sig.Params = append(sig.Params, pt)
		if name != "" && !pt.IsEmpty() {
			locals[name] = pt
		}
	}
	inner := ctx
	inner.Locals = locals
	var ret ResolvedType
	switch {
	case declaredReturn != "":
		ret = r.resolveDocType(declaredReturn, ctx.File)
	case arrow && strings.HasPrefix(rest, "=>"):
		ret = r.ExpressionType(strings.TrimSpace(rest[2:]), inner)
	case !arrow && strings.HasPrefix(rest, "{"):
		if body := firstReturnExpr(rest); body != "" {
			ret = r.ExpressionType(body, inner)
		}
	}
	if !ret.IsEmpty() || ret.Nullable {
		sig.Return = &ret
	}
	return ResolvedType{FQN: "Closure", Callable: sig}
}

// closureParamParts splits `?User &$user = null` into $user and ?User.
func closureParamParts(raw string) (name, declared string) {
	raw = strings.TrimSpace(raw)
	if eq := IndexOutsideBrackets(raw, '='); eq >= 0 {
		raw = strings.TrimSpace(raw[:eq])
	}
	dollar := strings.LastIndexByte(raw, '$')
	if dollar < 0 {
		return "", ""
	}
	name = raw[dollar:]
	declared = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(raw[:dollar]), "&."))
	// Constructor promotion and attributes don't occur in closures; a
	// leading modifier would be a parse error anyway.
	return name, declared
}

// firstReturnExpr returns the expression of the first return statement in a
// closure body that isn't inside a nested closure.
func firstReturnExpr(body string) string {
	depth := 0
	var quote byte
	for i := 0; i < len(body); i++ {
		c := body[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '{':
			depth++
		case '}':
			depth--
		case 'r':
			if depth == 1 && strings.HasPrefix(body[i:], "return") && (i == 0 || !isIdentByte(body[i-1])) &&
				i+6 < len(body) && !isIdentByte(body[i+6]) {
				rest := body[i+6:]
				end := IndexOutsideBrackets(rest, ';')
				if end < 0 {
					return ""
				}
				return strings.TrimSpace(rest[:end])
			}
		}
	}
	return ""
}
