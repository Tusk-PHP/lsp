package parser

import (
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type assoc uint8

const (
	left assoc = iota
	right
	nonassoc
)

const (
	bpLowest     = 0
	bpLogicalOr  = 1
	bpLogicalXor = 2
	bpLogicalAnd = 3
	bpPrint      = 4
	bpYield      = 5
	bpAssign     = 7
	bpTernary    = 8
	bpCoalesce   = 9
	bpBoolOr     = 10
	bpBoolAnd    = 11
	bpBitOr      = 12
	bpBitXor     = 13
	bpBitAnd     = 14
	bpEquality   = 15
	bpCompare    = 16
	bpPipe       = 17
	bpConcat     = 18
	bpShift      = 19
	bpAdd        = 20
	bpMul        = 21
	bpNot        = 22
	bpInstanceOf = 23
	bpUnary      = 24
	bpPow        = 25
	bpClone      = 26
)

func infix(k token.Kind) (int, assoc, bool) {
	switch k {
	case token.Or:
		return bpLogicalOr, left, true
	case token.Xor:
		return bpLogicalXor, left, true
	case token.And:
		return bpLogicalAnd, left, true
	case token.Question:
		return bpTernary, left, true
	case token.Coalesce:
		return bpCoalesce, right, true
	case token.BooleanOr:
		return bpBoolOr, left, true
	case token.BooleanAnd:
		return bpBoolAnd, left, true
	case token.BitwiseOr:
		return bpBitOr, left, true
	case token.BitwiseXor:
		return bpBitXor, left, true
	case token.Ampersand, token.AmpersandFollowedByVarOrVararg, token.AmpersandNotFollowedByVarOrVararg:
		return bpBitAnd, left, true
	case token.IsEqual, token.IsNotEqual, token.IsIdentical, token.IsNotIdentical, token.Spaceship:
		return bpEquality, nonassoc, true
	case token.LessThan, token.IsSmallerOrEqual, token.GreaterThan, token.IsGreaterOrEqual:
		return bpCompare, nonassoc, true
	case token.Pipe:
		return bpPipe, left, true
	case token.Dot:
		return bpConcat, left, true
	case token.ShiftLeft, token.ShiftRight:
		return bpShift, left, true
	case token.Plus, token.Minus:
		return bpAdd, left, true
	case token.Asterisk, token.Slash, token.Percent:
		return bpMul, left, true
	case token.InstanceOf:
		return bpInstanceOf, nonassoc, true
	case token.Pow:
		return bpPow, right, true
	}
	return 0, left, false
}

func isAssignOp(k token.Kind) bool {
	switch k {
	case token.Equals, token.PlusEqual, token.MinusEqual, token.MulEqual, token.DivEqual, token.ConcatEqual,
		token.ModEqual, token.AndEqual, token.OrEqual, token.XorEqual, token.ShiftLeftEqual, token.ShiftRightEqual,
		token.CoalesceEqual, token.PowEqual:
		return true
	}
	return false
}

func isAssignable(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Variable, *ast.VariableVariable, *ast.PropertyFetch, *ast.StaticPropertyFetch, *ast.ArrayDimFetch, *ast.List, *ast.Array:
		return true
	}
	return false
}

func (p *parser) expr() ast.Expr {
	return p.exprBP(bpLowest)
}

func (p *parser) exprBP(min int) ast.Expr {
	start := p.start()
	lhs := p.unary()
	for {
		k := p.peek()
		if isAssignOp(k) && isAssignable(lhs) {
			lhs = p.assignment(lhs, start)
			continue
		}
		bp, as, ok := infix(k)
		if !ok || bp < min {
			return lhs
		}
		p.next()
		switch {
		case k == token.Question:
			lhs = p.ternary(lhs, start)
		case k == token.InstanceOf:
			lhs = &ast.InstanceOf{Operand: lhs, Class: p.classReference()}
			lhs.(*ast.InstanceOf).Span = p.span(start)
		default:
			next := bp + 1
			if as == right {
				next = bp
			}
			rhs := p.exprBP(next)
			lhs = &ast.Binary{Span: p.span(start), Op: k, Left: lhs, Right: rhs}
			if as == nonassoc {
				if nbp, _, ok := infix(p.peek()); ok && nbp == bp {
					p.unexpected("this operator is non-associative")
				}
			}
		}
	}
}

func (p *parser) ternary(cond ast.Expr, start uint32) ast.Expr {
	var then ast.Expr
	if !p.accept(token.Colon) {
		then = p.expr()
		p.expect(token.Colon)
	}
	return &ast.Ternary{Cond: cond, Then: then, Else: p.exprBP(bpTernary + 1), Span: p.span(start)}
}

func (p *parser) assignment(target ast.Expr, start uint32) ast.Expr {
	op := p.next().Kind
	if arr, ok := target.(*ast.Array); ok && op == token.Equals {
		target = &ast.List{Span: arr.Span, Short: true, Items: arr.Items}
	}
	byRef := false
	if op == token.Equals && isAmpersand(p.peek()) {
		p.next()
		byRef = true
	}
	value := p.exprBP(bpAssign)
	return &ast.Assign{Span: p.span(start), Op: op, ByRef: byRef, Target: target, Value: value}
}

func isAmpersand(k token.Kind) bool {
	return k == token.Ampersand || k == token.AmpersandFollowedByVarOrVararg || k == token.AmpersandNotFollowedByVarOrVararg
}

func (p *parser) unary() ast.Expr {
	start := p.start()
	k := p.peek()
	switch k {
	case token.Bang:
		p.next()
		return &ast.Unary{Op: k, Operand: p.exprBP(bpNot), Span: p.span(start)}
	case token.Minus, token.Plus, token.Tilde, token.At:
		p.next()
		return &ast.Unary{Op: k, Operand: p.exprBP(bpUnary), Span: p.span(start)}
	case token.Increment, token.Decrement:
		p.next()
		return &ast.Unary{Op: k, Operand: p.exprBP(bpUnary), Span: p.span(start)}
	case token.IntCast, token.FloatCast, token.StringCast, token.ArrayCast, token.ObjectCast, token.BoolCast, token.UnsetCast:
		p.next()
		return &ast.Cast{Kind: k, Operand: p.exprBP(bpUnary), Span: p.span(start)}
	case token.Clone:
		return p.clone()
	case token.Print:
		p.next()
		return &ast.Print{Operand: p.exprBP(bpPrint + 1), Span: p.span(start)}
	case token.Yield:
		return p.yield()
	case token.YieldFrom:
		p.next()
		return &ast.YieldFrom{Operand: p.exprBP(bpYield + 1), Span: p.span(start)}
	case token.Throw:
		p.next()
		return &ast.Throw{Operand: p.expr(), Span: p.span(start)}
	case token.Include, token.IncludeOnce, token.Require, token.RequireOnce:
		p.next()
		return &ast.Include{Kind: k, Operand: p.expr(), Span: p.span(start)}
	}
	e := p.primary()
	if n, ok := e.(*ast.New); ok && (n.Args == nil || p.version < token.PHP84) {
		return e
	}
	return p.postfix(e, start)
}

func (p *parser) clone() ast.Expr {
	start := p.start()
	p.next()
	if p.version >= token.PHP85 && p.peek() == token.LeftParen {
		save, saveErrors := p.pos, len(p.errors)
		args := p.args()
		if len(args.Args) != 1 || args.RestPlaceholder || args.Args[0].Name != nil || args.Args[0].Unpack {
			return &ast.Clone{Args: args, Span: p.span(start)}
		}
		p.pos, p.errors = save, p.errors[:saveErrors]
	}
	return &ast.Clone{Operand: p.exprBP(bpClone), Span: p.span(start)}
}

func (p *parser) yield() ast.Expr {
	start := p.start()
	p.next()
	if !startsExpr(p.peek()) {
		return &ast.Yield{Span: p.span(start)}
	}
	value := p.exprBP(bpYield + 1)
	if p.accept(token.DoubleArrow) {
		return &ast.Yield{Key: value, Value: p.exprBP(bpYield + 1), Span: p.span(start)}
	}
	return &ast.Yield{Value: value, Span: p.span(start)}
}

func startsExpr(k token.Kind) bool {
	switch k {
	case eof, token.Semicolon, token.RightParen, token.RightBracket, token.RightBrace, token.Comma, token.DoubleArrow, token.CloseTag, token.Colon, token.As:
		return false
	}
	_, _, isInfix := infix(k)
	return !isInfix || k == token.Minus || k == token.Plus || isAmpersand(k)
}

func (p *parser) primary() ast.Expr {
	start := p.start()
	k := p.peek()
	switch k {
	case token.Variable:
		p.next()
		return &ast.Variable{Span: p.span(start)}
	case token.Dollar:
		return p.variableVariable()
	case token.IntegerLiteral, token.FloatLiteral, token.ConstantString:
		p.next()
		return &ast.Literal{Kind: k, Span: p.span(start)}
	case token.DoubleQuote, token.Backtick:
		return p.interpolated(k, k)
	case token.StartHeredoc:
		return p.interpolated(token.StartHeredoc, token.EndHeredoc)
	case token.ClassConst, token.DirConst, token.FileConst, token.FunctionConst, token.LineConst,
		token.MethodConst, token.NamespaceConst, token.PropertyConst, token.TraitConst:
		p.next()
		return &ast.MagicConst{Kind: k, Span: p.span(start)}
	case token.LeftParen:
		p.next()
		inner := p.expr()
		p.expect(token.RightParen)
		return p.postfix(inner, start)
	case token.LeftBracket:
		return p.array(true)
	case token.Array:
		if p.peekAt(1) == token.LeftParen {
			return p.array(false)
		}
	case token.List:
		return p.list()
	case token.Isset:
		p.next()
		p.expect(token.LeftParen)
		vars := p.exprList(token.RightParen)
		p.expect(token.RightParen)
		return &ast.Isset{Vars: vars, Span: p.span(start)}
	case token.Empty:
		p.next()
		p.expect(token.LeftParen)
		operand := p.expr()
		p.expect(token.RightParen)
		return &ast.Empty{Operand: operand, Span: p.span(start)}
	case token.Eval:
		p.next()
		p.expect(token.LeftParen)
		operand := p.expr()
		p.expect(token.RightParen)
		return &ast.Eval{Operand: operand, Span: p.span(start)}
	case token.Exit:
		p.next()
		var args *ast.ArgList
		if p.peek() == token.LeftParen {
			args = p.args()
		}
		return &ast.Exit{Args: args, Span: p.span(start)}
	case token.New:
		return p.newExpr()
	case token.Function, token.Fn:
		return p.closure(start, nil, false)
	case token.Static:
		if next := p.peekAt(1); next == token.Function || next == token.Fn {
			p.next()
			return p.closure(start, nil, true)
		}
		p.next()
		if p.peek() != token.DoubleColon {
			p.unexpected("expected '::'")
		}
		return &ast.ConstFetch{Name: &ast.Name{Span: p.span(start), Kind: token.Static}, Span: p.span(start)}
	case token.Attribute:
		attrs := p.attributes()
		static := p.accept(token.Static)
		if k := p.peek(); k == token.Function || k == token.Fn {
			return p.closure(start, attrs, static)
		}
		p.unexpected("expected a closure after attributes")
		return &ast.BadExpr{Span: p.span(start)}
	case token.Match:
		return p.match()
	case token.Readonly:
		if p.peekAt(1) == token.LeftParen {
			t := p.next()
			return &ast.ConstFetch{Name: &ast.Name{Span: ast.Span{Start: t.Start, End: t.End}, Kind: token.Identifier}, Span: p.span(start)}
		}
	case token.Identifier, token.NameQualified, token.NameFullyQualified, token.NameRelative:
		name := p.name()
		return &ast.ConstFetch{Name: name, Span: p.span(start)}
	}
	p.unexpected("expected an expression")
	if k != eof && k != token.Semicolon && k != token.RightBrace && k != token.RightParen && k != token.CloseTag {
		p.next()
	}
	return &ast.BadExpr{Span: p.span(start)}
}

func (p *parser) variableVariable() ast.Expr {
	start := p.start()
	p.next()
	if p.accept(token.LeftBrace) {
		name := p.expr()
		p.expect(token.RightBrace)
		return &ast.VariableVariable{Name: name, Span: p.span(start)}
	}
	if p.peek() == token.Variable || p.peek() == token.Dollar {
		return &ast.VariableVariable{Name: p.simpleVariable(), Span: p.span(start)}
	}
	p.unexpected("expected a variable")
	return &ast.BadExpr{Span: p.span(start)}
}

func (p *parser) simpleVariable() ast.Expr {
	if p.peek() == token.Dollar {
		return p.variableVariable()
	}
	start := p.start()
	if !p.expect(token.Variable) {
		return &ast.BadExpr{Span: p.span(start)}
	}
	return &ast.Variable{Span: p.span(start)}
}

func (p *parser) postfix(e ast.Expr, start uint32) ast.Expr {
	for {
		switch p.peek() {
		case token.LeftBracket:
			p.next()
			var dim ast.Expr
			if p.peek() != token.RightBracket {
				dim = p.expr()
			}
			p.expect(token.RightBracket)
			e = &ast.ArrayDimFetch{Array: e, Dim: dim, Span: p.span(start)}
		case token.ObjectOperator, token.NullsafeObjectOperator:
			nullsafe := p.next().Kind == token.NullsafeObjectOperator
			name := p.memberAccessName()
			if p.peek() == token.LeftParen {
				e = &ast.MethodCall{Object: e, Name: name, Nullsafe: nullsafe, Args: p.args(), Span: p.span(start)}
			} else {
				e = &ast.PropertyFetch{Object: e, Name: name, Nullsafe: nullsafe, Span: p.span(start)}
			}
		case token.DoubleColon:
			p.next()
			e = p.staticMember(e, start)
		case token.LeftParen:
			e = &ast.Call{Callee: e, Args: p.args(), Span: p.span(start)}
		case token.Increment, token.Decrement:
			op := p.next().Kind
			e = &ast.Postfix{Op: op, Operand: e, Span: p.span(start)}
		default:
			return e
		}
	}
}

func (p *parser) memberAccessName() ast.Expr {
	start := p.start()
	switch p.peek() {
	case token.Identifier:
		p.next()
		return &ast.IdentifierExpr{Span: p.span(start)}
	case token.Variable, token.Dollar:
		return p.simpleVariable()
	case token.LeftBrace:
		p.next()
		name := p.expr()
		p.expect(token.RightBrace)
		return name
	}
	p.unexpected("expected a property or method name")
	return &ast.BadExpr{Span: p.span(start)}
}

func (p *parser) staticMember(class ast.Expr, start uint32) ast.Expr {
	nameStart := p.start()
	switch k := p.peek(); {
	case k == token.Variable || k == token.Dollar:
		name := p.simpleVariable()
		if p.peek() == token.LeftParen {
			return &ast.StaticCall{Class: class, Name: name, Args: p.args(), Span: p.span(start)}
		}
		return &ast.StaticPropertyFetch{Class: class, Name: name, Span: p.span(start)}
	case k == token.LeftBrace:
		p.next()
		name := p.expr()
		p.expect(token.RightBrace)
		if p.peek() == token.LeftParen {
			return &ast.StaticCall{Class: class, Name: name, Args: p.args(), Span: p.span(start)}
		}
		p.requires(token.PHP83, nameStart, "dynamic class constant fetch")
		return &ast.ClassConstFetch{Class: class, Name: name, Span: p.span(start)}
	case isSemiReserved(k):
		p.next()
		name := &ast.IdentifierExpr{Span: p.span(nameStart)}
		if p.peek() == token.LeftParen {
			return &ast.StaticCall{Class: class, Name: name, Args: p.args(), Span: p.span(start)}
		}
		return &ast.ClassConstFetch{Class: class, Name: name, Span: p.span(start)}
	}
	p.unexpected("expected a class member")
	return &ast.BadExpr{Span: p.span(start)}
}

func (p *parser) args() *ast.ArgList {
	start := p.start()
	p.expect(token.LeftParen)
	list := &ast.ArgList{}
	for p.peek() != token.RightParen && p.peek() != eof {
		if p.peek() == token.Ellipsis && p.peekAt(1) == token.RightParen {
			restStart := p.start()
			p.next()
			if len(list.Args) == 0 {
				p.requires(token.PHP81, restStart, "first-class callable syntax")
			} else {
				p.requires(token.PHP86, restStart, "partial function application")
			}
			list.RestPlaceholder = true
			break
		}
		list.Args = append(list.Args, p.arg())
		if !p.accept(token.Comma) {
			break
		}
	}
	p.expect(token.RightParen)
	list.Span = p.span(start)
	return list
}

func (p *parser) arg() *ast.Arg {
	start := p.start()
	arg := &ast.Arg{}
	switch {
	case p.accept(token.Ellipsis):
		arg.Unpack = true
	case isSemiReserved(p.peek()) && p.peekAt(1) == token.Colon:
		arg.Name = p.memberName()
		p.next()
	}
	if !arg.Unpack && p.peek() == token.Question {
		if next := p.peekAt(1); next == token.Comma || next == token.RightParen {
			p.next()
			p.requires(token.PHP86, start, "partial function application")
			arg.Placeholder = true
			arg.Span = p.span(start)
			return arg
		}
	}
	arg.Value = p.expr()
	arg.Span = p.span(start)
	return arg
}

func (p *parser) exprList(end token.Kind) []ast.Expr {
	var exprs []ast.Expr
	for p.peek() != end && p.peek() != eof {
		exprs = append(exprs, p.expr())
		if !p.accept(token.Comma) {
			break
		}
	}
	return exprs
}

func (p *parser) array(short bool) ast.Expr {
	start := p.start()
	end := token.RightBracket
	if !short {
		p.next()
		end = token.RightParen
	}
	p.next()
	items := p.arrayItems(end)
	p.expect(end)
	return &ast.Array{Short: short, Items: items, Span: p.span(start)}
}

func (p *parser) list() ast.Expr {
	start := p.start()
	p.next()
	p.expect(token.LeftParen)
	items := p.arrayItems(token.RightParen)
	p.expect(token.RightParen)
	return &ast.List{Items: items, Span: p.span(start)}
}

func (p *parser) arrayItems(end token.Kind) []*ast.ArrayItem {
	var items []*ast.ArrayItem
	for p.peek() != end && p.peek() != eof {
		if p.peek() == token.Comma {
			p.next()
			items = append(items, nil)
			continue
		}
		items = append(items, p.arrayItem())
		if !p.accept(token.Comma) {
			break
		}
	}
	return items
}

func (p *parser) arrayItem() *ast.ArrayItem {
	start := p.start()
	item := &ast.ArrayItem{}
	if p.accept(token.Ellipsis) {
		item.Unpack = true
		item.Value = p.expr()
		item.Span = p.span(start)
		return item
	}
	if isAmpersand(p.peek()) {
		p.next()
		item.ByRef = true
		item.Value = p.expr()
		item.Span = p.span(start)
		return item
	}
	value := p.expr()
	if p.accept(token.DoubleArrow) {
		item.Key = value
		if isAmpersand(p.peek()) {
			p.next()
			item.ByRef = true
		}
		value = p.expr()
	}
	item.Value = value
	item.Span = p.span(start)
	return item
}

func (p *parser) newExpr() ast.Expr {
	start := p.start()
	p.next()
	n := &ast.New{}
	if p.peek() == token.Class || p.peek() == token.Attribute || p.peek() == token.Readonly {
		n.Anonymous = p.anonymousClass(&n.Args)
		n.Span = p.span(start)
		return n
	}
	n.Class = p.classReference()
	if p.peek() == token.LeftParen {
		n.Args = p.args()
	}
	n.Span = p.span(start)
	return n
}

func (p *parser) classReference() ast.Expr {
	start := p.start()
	var e ast.Expr
	switch k := p.peek(); k {
	case token.Identifier, token.NameQualified, token.NameFullyQualified, token.NameRelative:
		e = &ast.ConstFetch{Name: p.name(), Span: p.span(start)}
	case token.Static:
		p.next()
		e = &ast.ConstFetch{Name: &ast.Name{Span: p.span(start), Kind: token.Static}, Span: p.span(start)}
	case token.LeftParen:
		p.next()
		inner := p.expr()
		p.expect(token.RightParen)
		return inner
	default:
		e = p.simpleVariable()
	}
	if _, named := e.(*ast.ConstFetch); named {
		if next := p.peekAt(1); p.peek() != token.DoubleColon || next != token.Variable && next != token.Dollar {
			return e
		}
	}
	for {
		start := e.Bounds().Start
		switch p.peek() {
		case token.LeftBracket:
			p.next()
			dim := p.expr()
			p.expect(token.RightBracket)
			e = &ast.ArrayDimFetch{Array: e, Dim: dim, Span: p.span(start)}
		case token.ObjectOperator, token.NullsafeObjectOperator:
			nullsafe := p.next().Kind == token.NullsafeObjectOperator
			e = &ast.PropertyFetch{Object: e, Name: p.memberAccessName(), Nullsafe: nullsafe, Span: p.span(start)}
		case token.DoubleColon:
			p.next()
			e = &ast.StaticPropertyFetch{Class: e, Name: p.simpleVariable(), Span: p.span(start)}
		default:
			return e
		}
	}
}

func (p *parser) closure(start uint32, attrs []*ast.AttributeGroup, static bool) ast.Expr {
	arrow := p.next().Kind == token.Fn
	byRef := false
	if isAmpersand(p.peek()) {
		p.next()
		byRef = true
	}
	params := p.params()
	if arrow {
		returnType := p.returnType()
		p.expect(token.DoubleArrow)
		body := p.expr()
		return &ast.ArrowFunction{Attrs: attrs, Static: static, ByRef: byRef, Params: params, ReturnType: returnType, Body: body, Span: p.span(start)}
	}
	var uses []*ast.ClosureUse
	if p.accept(token.Use) {
		p.expect(token.LeftParen)
		for p.peek() != token.RightParen && p.peek() != eof {
			useStart := p.start()
			use := &ast.ClosureUse{}
			if isAmpersand(p.peek()) {
				p.next()
				use.ByRef = true
			}
			varStart := p.start()
			if p.expect(token.Variable) {
				use.Var = &ast.Variable{Span: p.span(varStart)}
			}
			use.Span = p.span(useStart)
			uses = append(uses, use)
			if !p.accept(token.Comma) {
				break
			}
		}
		p.expect(token.RightParen)
	}
	returnType := p.returnType()
	body := p.block()
	return &ast.Closure{Attrs: attrs, Static: static, ByRef: byRef, Params: params, Uses: uses, ReturnType: returnType, Body: body, Span: p.span(start)}
}

func (p *parser) match() ast.Expr {
	start := p.start()
	p.next()
	p.expect(token.LeftParen)
	subject := p.expr()
	p.expect(token.RightParen)
	p.expect(token.LeftBrace)
	m := &ast.Match{Subject: subject}
	for p.peek() != token.RightBrace && p.peek() != eof {
		armStart := p.start()
		arm := &ast.MatchArm{}
		if p.accept(token.Default) {
			p.accept(token.Comma)
		} else {
			arm.Conds = p.exprList(token.DoubleArrow)
		}
		p.expect(token.DoubleArrow)
		arm.Body = p.expr()
		arm.Span = p.span(armStart)
		m.Arms = append(m.Arms, arm)
		if !p.accept(token.Comma) {
			break
		}
	}
	p.expect(token.RightBrace)
	m.Span = p.span(start)
	return m
}

func (p *parser) interpolated(open, close token.Kind) ast.Expr {
	start := p.start()
	opening := p.next()
	s := &ast.InterpolatedString{Kind: open}
	for k := p.peek(); k != close && k != eof; k = p.peek() {
		partStart := p.start()
		switch k {
		case token.EncapsedAndWhitespace:
			p.next()
			s.Parts = append(s.Parts, &ast.StringPart{Span: p.span(partStart)})
		case token.Variable:
			s.Parts = append(s.Parts, p.variableInString())
		case token.CurlyOpen:
			p.next()
			s.Parts = append(s.Parts, p.expr())
			p.expect(token.RightBrace)
		case token.DollarOpenCurlyBraces:
			p.next()
			s.Parts = append(s.Parts, p.dollarBraces(partStart))
		default:
			p.unexpected("expected string content")
			p.next()
		}
	}
	if p.peek() == eof {
		p.unterminatedAt(ast.Span{Start: opening.Start, End: opening.End}, "unterminated string")
	} else {
		p.expect(close)
	}
	s.Span = p.span(start)
	return s
}

func (p *parser) variableInString() ast.Expr {
	start := p.start()
	p.next()
	var e ast.Expr = &ast.Variable{Span: p.span(start)}
	switch p.peek() {
	case token.LeftBracket:
		p.next()
		dimStart := p.start()
		var dim ast.Expr
		switch k := p.peek(); k {
		case token.Identifier, token.NumString:
			p.next()
			dim = &ast.Literal{Kind: k, Span: p.span(dimStart)}
		case token.Minus:
			p.next()
			p.expect(token.NumString)
			dim = &ast.Literal{Kind: token.NumString, Span: p.span(dimStart)}
		case token.Variable:
			p.next()
			dim = &ast.Variable{Span: p.span(dimStart)}
		default:
			p.unexpected("expected an array offset")
		}
		p.expect(token.RightBracket)
		e = &ast.ArrayDimFetch{Array: e, Dim: dim, Span: p.span(start)}
	case token.ObjectOperator, token.NullsafeObjectOperator:
		nullsafe := p.next().Kind == token.NullsafeObjectOperator
		nameStart := p.start()
		p.expect(token.Identifier)
		e = &ast.PropertyFetch{Object: e, Name: &ast.IdentifierExpr{Span: p.span(nameStart)}, Nullsafe: nullsafe, Span: p.span(start)}
	}
	return e
}

func (p *parser) dollarBraces(start uint32) ast.Expr {
	if p.peek() == token.StringVarname {
		nameStart := p.start()
		p.next()
		var e ast.Expr = &ast.VariableVariable{Name: &ast.Literal{Kind: token.StringVarname, Span: p.span(nameStart)}, Span: p.span(start)}
		if p.accept(token.LeftBracket) {
			dim := p.expr()
			p.expect(token.RightBracket)
			e = &ast.ArrayDimFetch{Array: e, Dim: dim, Span: p.span(start)}
		}
		p.expect(token.RightBrace)
		return e
	}
	name := p.expr()
	p.expect(token.RightBrace)
	return &ast.VariableVariable{Name: name, Span: p.span(start)}
}
