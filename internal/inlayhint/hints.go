package inlayhint

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type hinter struct {
	index   *symbols.Index
	r       *resolve.Resolver
	file    *parser.FileNode
	lines   []string
	offsets ast.Lines
	cfg     *config.InlayHintsConfig
	hints   []protocol.InlayHint
}

func (h *hinter) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.Assign:
		if v, ok := n.Target.(*ast.Variable); ok && n.Op == token.Equals && h.cfg.VariableTypes {
			h.variableHint(v, n.Value)
		}
	case *ast.StaticVar:
		if n.Default != nil && h.cfg.VariableTypes {
			h.variableHint(n.Var, n.Default)
		}
	case *ast.Foreach:
		if h.cfg.ForeachTypes {
			h.foreachHints(n)
		}
	case *ast.Closure:
		if n.ReturnType == nil && n.Body != nil && h.cfg.ClosureReturnTypes {
			h.closureReturnHint(n.Span, n.Attrs, n.Body.Start, h.firstReturnType(n.Body))
		}
	case *ast.ArrowFunction:
		if n.ReturnType == nil && n.Body != nil && h.cfg.ClosureReturnTypes {
			h.closureReturnHint(n.Span, n.Attrs, n.Body.Bounds().Start, h.valueType(n.Body))
		}
	case *ast.Method:
		if n.ReturnType == nil && h.cfg.ReturnTypes {
			h.methodReturnHint(n)
		}
	case *ast.Call:
		if name, ok := n.Callee.(*ast.ConstFetch); ok && h.cfg.ParameterNames {
			if fn := h.r.CalledFunction(h.text(name), h.file); fn != nil {
				h.parameterHints(fn.Params, n.Args)
			}
		}
	case *ast.MethodCall:
		if h.cfg.ParameterNames {
			h.memberCallHints(n.Object, n.Name, n.Args)
		}
	case *ast.StaticCall:
		if h.cfg.ParameterNames {
			h.memberCallHints(n.Class, n.Name, n.Args)
		}
	}
	return true
}

func (h *hinter) text(n ast.Node) string {
	return n.Bounds().Text(h.file.Source)
}

func (h *hinter) position(off uint32) protocol.Position {
	line, col := h.offsets.Position(off)
	return protocol.Position{Line: line, Character: col}
}

func (h *hinter) typeHint(at uint32, rt resolve.ResolvedType) {
	label := shortenFQNs(rt.String())
	if label == "" || label == "mixed" {
		return
	}
	h.hints = append(h.hints, protocol.InlayHint{
		Position:    h.position(at),
		Label:       ": " + label,
		Kind:        protocol.InlayHintKindType,
		PaddingLeft: true,
	})
}

func (h *hinter) variableTypeAfter(v *ast.Variable, line int) resolve.ResolvedType {
	return h.r.ResolveVariableTypeTyped(h.text(v), h.lines, protocol.Position{Line: line + 1}, h.file)
}

func (h *hinter) variableHint(v *ast.Variable, value ast.Expr) {
	line, _ := h.offsets.Position(v.Start)
	if rt := h.variableTypeAfter(v, line); !h.states(rt, value) {
		h.typeHint(v.End, rt)
	}
}

func (h *hinter) foreachHints(n *ast.Foreach) {
	if n.Body == nil {
		return
	}
	line, _ := h.offsets.Position(n.Body.Bounds().Start)
	if k, ok := n.Key.(*ast.Variable); ok {
		h.typeHint(k.End, h.variableTypeAfter(k, line))
	}
	if v, ok := n.Value.(*ast.Variable); ok && !n.ByRef {
		h.typeHint(v.End, h.variableTypeAfter(v, line))
	}
}

type returned struct {
	value ast.Expr
	rt    resolve.ResolvedType
}

func (h *hinter) valueType(e ast.Expr) returned {
	return returned{value: e, rt: h.r.NodeType(h.file, e)}
}

func (h *hinter) firstReturnType(body *ast.Block) returned {
	for _, stmt := range body.Stmts {
		if ret, ok := stmt.(*ast.Return); ok && ret.Value != nil {
			if v := h.valueType(ret.Value); !v.rt.IsEmpty() {
				return v
			}
		}
	}
	return returned{}
}

func (h *hinter) closureReturnHint(span ast.Span, attrs []*ast.AttributeGroup, bodyStart uint32, ret returned) {
	if ret.rt.IsEmpty() || h.states(ret.rt, ret.value) {
		return
	}
	from := span.Start
	if len(attrs) > 0 {
		from = attrs[len(attrs)-1].End
	}
	if end, ok := h.paramListEnd(from, bodyStart); ok {
		h.typeHint(end, ret.rt)
	}
}

func (h *hinter) methodReturnHint(m *ast.Method) {
	doc := parser.ParseDocBlock(h.text(m.Doc))
	if doc == nil {
		return
	}
	switch ret := shortenFQNs(doc.Return.Type); ret {
	case "", "void", "mixed":
	default:
		to := m.End
		if m.Body != nil {
			to = m.Body.Start
		}
		if end, ok := h.paramListEnd(m.Name.End, to); ok {
			h.hints = append(h.hints, protocol.InlayHint{
				Position:    h.position(end),
				Label:       ": " + ret,
				Kind:        protocol.InlayHintKindType,
				PaddingLeft: true,
			})
		}
	}
}

func (h *hinter) paramListEnd(from, to uint32) (uint32, bool) {
	depth := 0
	for _, tok := range lexer.LexCode(h.file.Source[from:to], h.file.Grammar) {
		switch tok.Kind {
		case token.LeftParen:
			depth++
		case token.RightParen:
			depth--
			if depth == 0 {
				return from + tok.End, true
			}
		}
	}
	return 0, false
}

func (h *hinter) memberCallHints(receiver, name ast.Expr, args *ast.ArgList) {
	method, ok := name.(*ast.IdentifierExpr)
	if !ok {
		return
	}
	rt := h.r.NodeType(h.file, receiver)
	if rt.IsEmpty() {
		return
	}
	methodName := h.text(method)
	for _, sym := range h.index.GetClassMembers(rt.BaseFQN()) {
		if sym.Name == methodName {
			h.parameterHints(sym.Params, args)
			return
		}
	}
}

func (h *hinter) parameterHints(params []symbols.ParamInfo, args *ast.ArgList) {
	if len(params) == 0 || h.cfg.SuppressSingleParam && len(params) == 1 {
		return
	}
	for i, arg := range args.Args {
		if i >= len(params) || params[i].IsVariadic {
			return
		}
		label := strings.TrimPrefix(params[i].Name, "$")
		if _, missing := arg.Value.(*ast.BadExpr); missing || arg.Name != nil {
			continue
		}
		if v, ok := arg.Value.(*ast.Variable); ok && h.cfg.SuppressNameMatch && h.text(v) == "$"+label {
			continue
		}
		h.hints = append(h.hints, protocol.InlayHint{
			Position:     h.position(arg.Start),
			Label:        label + ":",
			Kind:         protocol.InlayHintKindParameter,
			PaddingRight: true,
		})
	}
}

func (h *hinter) states(rt resolve.ResolvedType, value ast.Expr) bool {
	switch rt.String() {
	case "int", "string", "bool", "float", "null":
		return h.obvious(value)
	}
	return false
}

func (h *hinter) obvious(e ast.Expr) bool {
	if e.Parenthesized() {
		return true
	}
	switch n := e.(type) {
	case *ast.Literal, *ast.InterpolatedString, *ast.Cast:
		return true
	case *ast.Binary:
		return h.obvious(n.Left)
	case *ast.ConstFetch:
		switch strings.ToLower(h.text(n)) {
		case "true", "false", "null":
			return true
		}
	}
	return false
}

func shortenFQNs(s string) string {
	var sb strings.Builder
	runStart := -1
	flush := func(end int) {
		if runStart < 0 {
			return
		}
		run := s[runStart:end]
		if idx := strings.LastIndexByte(run, '\\'); idx >= 0 {
			run = run[idx+1:]
		}
		sb.WriteString(run)
		runStart = -1
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isRun := c == '\\' || c == '_' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if isRun {
			if runStart < 0 {
				runStart = i
			}
			continue
		}
		flush(i)
		sb.WriteByte(c)
	}
	flush(len(s))
	return sb.String()
}
