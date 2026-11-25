package parser

import (
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func (p *parser) statements(stop func(token.Kind) bool) []ast.Stmt {
	from := p.stmtStack.mark()
	for k := p.peek(); k != eof && k != token.RightBrace && !stop(k); k = p.peek() {
		before := p.pos
		if s := p.statement(); s != nil {
			p.stmtStack.push(s)
		}
		p.ensureProgress(before)
	}
	return p.stmtStack.take(from)
}

func (p *parser) block() *ast.Block {
	start := p.start()
	if !p.expect(token.LeftBrace) {
		return nil
	}
	stmts := p.statements(noStop)
	p.expect(token.RightBrace)
	return &ast.Block{Stmts: stmts, Span: p.span(start)}
}

func (p *parser) endStatement() {
	if p.accept(token.Semicolon) {
		return
	}
	p.unexpected("expected ';'")
	p.skipTo(func(k token.Kind) bool { return k == token.Semicolon || k == token.CloseTag })
	p.accept(token.Semicolon)
}

func (p *parser) statement() ast.Stmt {
	start := p.start()
	if !p.enter() {
		return &ast.BadStmt{Span: p.span(start)}
	}
	s := p.statementBody()
	p.depth--
	return s
}

func (p *parser) statementBody() ast.Stmt {
	start, doc := p.start(), p.doc()
	switch k := p.peek(); k {
	case token.CloseTag:
		p.next()
		return nil
	case token.InlineHTML:
		p.next()
		return &ast.InlineHTML{Span: p.span(start)}
	case token.OpenTagWithEcho:
		p.next()
		exprs := p.exprList(token.CloseTag)
		p.endStatement()
		return &ast.Echo{Exprs: exprs, Span: p.span(start)}
	case token.LeftBrace:
		return p.block()
	case token.Semicolon:
		p.next()
		return &ast.Nop{Span: p.span(start)}
	case token.If:
		return p.ifStmt()
	case token.While:
		return p.whileStmt()
	case token.Do:
		return p.doWhile()
	case token.For:
		return p.forStmt()
	case token.Foreach:
		return p.foreachStmt()
	case token.Switch:
		return p.switchStmt()
	case token.Break, token.Continue:
		p.next()
		var levels ast.Expr
		if p.peek() != token.Semicolon && p.peek() != token.CloseTag {
			levels = p.expr()
		}
		p.endStatement()
		if k == token.Break {
			return &ast.Break{Levels: levels, Span: p.span(start)}
		}
		return &ast.Continue{Levels: levels, Span: p.span(start)}
	case token.Throw:
		if p.version < token.PHP80 {
			p.next()
			e := &ast.Throw{Operand: p.expr()}
			e.Span = p.span(start)
			p.endStatement()
			return &ast.ExprStmt{Expr: e, Span: p.span(start)}
		}
	case token.Return:
		p.next()
		var value ast.Expr
		if p.peek() != token.Semicolon && p.peek() != token.CloseTag {
			value = p.expr()
		}
		p.endStatement()
		return &ast.Return{Value: value, Span: p.span(start)}
	case token.Global:
		p.next()
		vars := []ast.Expr{p.simpleVariable()}
		for p.accept(token.Comma) {
			vars = append(vars, p.simpleVariable())
		}
		p.endStatement()
		return &ast.Global{Vars: vars, Span: p.span(start)}
	case token.Static:
		if p.peekAt(1) == token.Variable {
			return p.staticVars()
		}
	case token.Echo:
		p.next()
		exprs := []ast.Expr{p.expr()}
		for p.accept(token.Comma) {
			exprs = append(exprs, p.expr())
		}
		p.endStatement()
		return &ast.Echo{Exprs: exprs, Span: p.span(start)}
	case token.Unset:
		p.next()
		p.expect(token.LeftParen)
		vars := p.exprList(token.RightParen)
		p.expect(token.RightParen)
		p.endStatement()
		return &ast.Unset{Vars: vars, Span: p.span(start)}
	case token.Try:
		return p.tryStmt()
	case token.Goto:
		p.next()
		label := p.identifier()
		p.endStatement()
		return &ast.Goto{Label: label, Span: p.span(start)}
	case token.Declare:
		return p.declare()
	case token.HaltCompiler:
		p.next()
		p.expect(token.LeftParen)
		p.expect(token.RightParen)
		p.endStatement()
		p.pos = len(p.toks)
		return &ast.HaltCompiler{Span: p.span(start)}
	case token.Namespace:
		if p.nameTokens() == 0 {
			return p.namespace()
		}
	case token.Use:
		return p.use()
	case token.Const:
		return p.constStmt(start, nil)
	case token.Function:
		if p.isFunctionDecl() {
			return p.functionDecl(start, doc, nil)
		}
	case token.Abstract, token.Final, token.Class:
		return p.classDecl(start, doc, nil)
	case token.Readonly:
		if p.peekAt(1) != token.LeftParen {
			return p.classDecl(start, doc, nil)
		}
	case token.Interface:
		return p.interfaceDecl(start, doc, nil)
	case token.Trait:
		return p.traitDecl(start, doc, nil)
	case token.Enum:
		return p.enumDecl(start, doc, nil)
	case token.Attribute:
		return p.attributedDecl(start, doc)
	case token.Identifier:
		if p.peekAt(1) == token.Colon {
			name := p.identifier()
			p.next()
			return &ast.Label{Name: name, Span: p.span(start)}
		}
	case token.VoidCast:
		e := p.voidCast()
		p.endStatement()
		return &ast.ExprStmt{Expr: e, Span: p.span(start)}
	}
	e := p.expr()
	p.endStatement()
	return &ast.ExprStmt{Expr: e, Span: p.span(start)}
}

func (p *parser) isFunctionDecl() bool {
	next := p.peekAt(1)
	if isAmpersand(next) {
		next = p.peekAt(2)
	}
	return next == token.Identifier || next == token.Readonly
}

func (p *parser) attributedDecl(start uint32, doc ast.Span) ast.Stmt {
	attrsAt := p.pos
	attrs := p.attributes()
	doc = p.docSince(attrsAt)
	switch p.peek() {
	case token.Function:
		if p.isFunctionDecl() {
			return p.functionDecl(start, doc, attrs)
		}
	case token.Abstract, token.Final, token.Class, token.Readonly:
		return p.classDecl(start, doc, attrs)
	case token.Interface:
		return p.interfaceDecl(start, doc, attrs)
	case token.Trait:
		return p.traitDecl(start, doc, attrs)
	case token.Enum:
		return p.enumDecl(start, doc, attrs)
	case token.Const:
		p.requires(token.PHP85, start, "attributes on constants")
		return p.constStmt(start, attrs)
	}
	if !p.enter() {
		return &ast.ExprStmt{Expr: &ast.BadExpr{Span: p.span(start)}, Span: p.span(start)}
	}
	e := p.operators(p.attributedClosure(start, attrs), bpLowest, start)
	p.depth--
	p.endStatement()
	return &ast.ExprStmt{Expr: e, Span: p.span(start)}
}

func (p *parser) constStmt(start uint32, attrs []*ast.AttributeGroup) ast.Stmt {
	p.next()
	items := p.constItems(p.identifier)
	p.endStatement()
	return &ast.Const{Attrs: attrs, Items: items, Span: p.span(start)}
}

func (p *parser) staticVars() ast.Stmt {
	start := p.start()
	p.next()
	s := &ast.Static{}
	for {
		varStart := p.start()
		p.next()
		v := &ast.StaticVar{Var: &ast.Variable{Span: p.span(varStart)}}
		if p.accept(token.Equals) {
			v.Default = p.expr()
		}
		v.Span = p.span(varStart)
		s.Vars = append(s.Vars, v)
		if !p.accept(token.Comma) || p.peek() != token.Variable {
			break
		}
	}
	p.endStatement()
	s.Span = p.span(start)
	return s
}

func (p *parser) condition() ast.Expr {
	p.expect(token.LeftParen)
	cond := p.expr()
	p.expect(token.RightParen)
	return cond
}

func isAlternativeEnd(ends ...token.Kind) func(token.Kind) bool {
	return func(k token.Kind) bool {
		for _, end := range ends {
			if k == end {
				return true
			}
		}
		return false
	}
}

func (p *parser) altBody(ends ...token.Kind) ast.Stmt {
	start := p.start()
	p.next()
	stmts := p.statements(isAlternativeEnd(ends...))
	return &ast.Block{Stmts: stmts, Span: p.span(start)}
}

func (p *parser) closeAlternative(end token.Kind) {
	p.expect(end)
	p.endStatement()
}

func (p *parser) ifStmt() ast.Stmt {
	start := p.start()
	p.next()
	s := &ast.If{Cond: p.condition()}
	if p.peek() == token.Colon {
		s.Then = p.altBody(token.ElseIf, token.Else, token.EndIf)
		for p.peek() == token.ElseIf {
			elseStart := p.start()
			p.next()
			cond := p.condition()
			if p.peek() != token.Colon {
				p.unexpected("expected ':'")
			}
			s.ElseIfs = append(s.ElseIfs, &ast.ElseIf{Cond: cond, Body: p.altBody(token.ElseIf, token.Else, token.EndIf), Span: p.span(elseStart)})
		}
		if p.accept(token.Else) {
			if p.peek() != token.Colon {
				p.unexpected("expected ':'")
			}
			s.Else = p.altBody(token.EndIf)
		}
		p.closeAlternative(token.EndIf)
		s.Span = p.span(start)
		return s
	}
	s.Then = p.statement()
	for {
		switch {
		case p.peek() == token.ElseIf:
			elseStart := p.start()
			p.next()
			cond := p.condition()
			s.ElseIfs = append(s.ElseIfs, &ast.ElseIf{Cond: cond, Body: p.statement(), Span: p.span(elseStart)})
			continue
		case p.accept(token.Else):
			s.Else = p.statement()
		}
		break
	}
	s.Span = p.span(start)
	return s
}

func (p *parser) whileStmt() ast.Stmt {
	start := p.start()
	p.next()
	s := &ast.While{Cond: p.condition()}
	if p.peek() == token.Colon {
		s.Body = p.altBody(token.EndWhile)
		p.closeAlternative(token.EndWhile)
	} else {
		s.Body = p.statement()
	}
	s.Span = p.span(start)
	return s
}

func (p *parser) doWhile() ast.Stmt {
	start := p.start()
	p.next()
	s := &ast.DoWhile{Body: p.statement()}
	p.expect(token.While)
	s.Cond = p.condition()
	p.endStatement()
	s.Span = p.span(start)
	return s
}

func (p *parser) forStmt() ast.Stmt {
	start := p.start()
	p.next()
	s := &ast.For{}
	p.expect(token.LeftParen)
	s.Init = p.forExprs(token.Semicolon, true)
	p.expect(token.Semicolon)
	s.Cond = p.forExprs(token.Semicolon, false)
	p.expect(token.Semicolon)
	s.Loop = p.forExprs(token.RightParen, true)
	p.expect(token.RightParen)
	if p.peek() == token.Colon {
		s.Body = p.altBody(token.EndFor)
		p.closeAlternative(token.EndFor)
	} else {
		s.Body = p.statement()
	}
	s.Span = p.span(start)
	return s
}

func (p *parser) voidCast() ast.Expr {
	start := p.start()
	p.next()
	return &ast.Cast{Kind: token.VoidCast, Operand: p.expr(), Span: p.span(start)}
}

func (p *parser) forExprs(end token.Kind, voidLast bool) []ast.Expr {
	var exprs []ast.Expr
	for p.peek() != end && p.peek() != eof {
		void := p.peek() == token.VoidCast
		if void {
			exprs = append(exprs, p.voidCast())
		} else {
			exprs = append(exprs, p.expr())
		}
		if p.accept(token.Comma) {
			continue
		}
		if void && !voidLast {
			p.unexpected("expected ','")
		}
		break
	}
	return exprs
}

func (p *parser) foreachStmt() ast.Stmt {
	start := p.start()
	p.next()
	s := &ast.Foreach{}
	p.expect(token.LeftParen)
	s.Subject = p.expr()
	p.expect(token.As)
	value, byRef := p.foreachTarget()
	if p.accept(token.DoubleArrow) {
		s.Key = value
		value, byRef = p.foreachTarget()
	}
	s.Value, s.ByRef = value, byRef
	p.expect(token.RightParen)
	if p.peek() == token.Colon {
		s.Body = p.altBody(token.EndForeach)
		p.closeAlternative(token.EndForeach)
	} else {
		s.Body = p.statement()
	}
	s.Span = p.span(start)
	return s
}

func (p *parser) foreachTarget() (ast.Expr, bool) {
	if isAmpersand(p.peek()) {
		p.next()
		return p.exprBP(bpAssign + 1), true
	}
	return destructuring(p.exprBP(bpAssign + 1)), false
}

func (p *parser) switchStmt() ast.Stmt {
	start := p.start()
	p.next()
	s := &ast.Switch{Subject: p.condition()}
	alternative := p.peek() == token.Colon
	if alternative {
		p.next()
	} else {
		p.expect(token.LeftBrace)
	}
	p.accept(token.Semicolon)
	for k := p.peek(); k == token.Case || k == token.Default; k = p.peek() {
		caseStart := p.start()
		p.next()
		c := &ast.Case{}
		if k == token.Case {
			c.Cond = p.expr()
		}
		if !p.accept(token.Colon) && !p.accept(token.Semicolon) {
			p.unexpected("expected ':'")
		}
		c.Stmts = p.statements(isAlternativeEnd(token.Case, token.Default, token.EndSwitch))
		c.Span = p.span(caseStart)
		s.Cases = append(s.Cases, c)
	}
	if alternative {
		p.closeAlternative(token.EndSwitch)
	} else {
		p.expect(token.RightBrace)
	}
	s.Span = p.span(start)
	return s
}

func (p *parser) tryStmt() ast.Stmt {
	start := p.start()
	p.next()
	s := &ast.Try{Body: p.block()}
	for p.peek() == token.Catch {
		catchStart := p.start()
		p.next()
		p.expect(token.LeftParen)
		c := &ast.Catch{Types: []*ast.Name{p.name()}}
		for p.accept(token.BitwiseOr) {
			c.Types = append(c.Types, p.name())
		}
		if p.peek() == token.Variable {
			varStart := p.start()
			p.next()
			c.Var = &ast.Variable{Span: p.span(varStart)}
		} else {
			p.requires(token.PHP80, catchStart, "catching an exception without a variable")
		}
		p.expect(token.RightParen)
		c.Body = p.block()
		c.Span = p.span(catchStart)
		s.Catches = append(s.Catches, c)
	}
	if p.accept(token.Finally) {
		s.Finally = p.block()
	}
	s.Span = p.span(start)
	return s
}

func (p *parser) declare() ast.Stmt {
	start := p.start()
	p.next()
	d := &ast.Declare{}
	p.expect(token.LeftParen)
	for p.peek() != token.RightParen && p.peek() != eof {
		dirStart := p.start()
		dir := &ast.DeclareDirective{Name: p.identifier()}
		p.expect(token.Equals)
		dir.Value = p.expr()
		dir.Span = p.span(dirStart)
		d.Directives = append(d.Directives, dir)
		if !p.accept(token.Comma) {
			break
		}
	}
	p.expect(token.RightParen)
	switch {
	case p.peek() == token.Colon:
		d.Body = p.altBody(token.EndDeclare)
		p.closeAlternative(token.EndDeclare)
	case p.peek() == token.Semicolon || p.peek() == token.CloseTag:
		p.endStatement()
	default:
		d.Body = p.statement()
	}
	d.Span = p.span(start)
	return d
}

func (p *parser) namespace() ast.Stmt {
	start := p.start()
	p.next()
	n := &ast.Namespace{}
	if p.peek() != token.LeftBrace {
		n.Name = p.nameOf(isDeclarableName)
	}
	if p.peek() == token.LeftBrace {
		n.Braced = true
		n.Stmts = p.block().Stmts
	} else {
		p.endStatement()
	}
	n.Span = p.span(start)
	return n
}

func (p *parser) use() ast.Stmt {
	start := p.start()
	p.next()
	u := &ast.Use{Kind: token.Use}
	if k := p.peek(); k == token.Function || k == token.Const {
		u.Kind = k
		p.next()
	}
	firstStart := p.start()
	first := p.nameOf(isImportableName)
	if p.accept(token.NamespaceSeparator) {
		u.Prefix = first
		p.expect(token.LeftBrace)
		for p.peek() != token.RightBrace && p.peek() != eof {
			itemStart := p.start()
			kind := u.Kind
			if k := p.peek(); u.Kind == token.Use && (k == token.Function || k == token.Const) {
				kind = k
				p.next()
			}
			u.Items = append(u.Items, p.useItem(itemStart, kind, p.nameOf(isDeclarableName)))
			if !p.accept(token.Comma) {
				break
			}
		}
		p.expect(token.RightBrace)
	} else {
		u.Items = append(u.Items, p.useItem(firstStart, u.Kind, first))
		for p.accept(token.Comma) {
			itemStart := p.start()
			u.Items = append(u.Items, p.useItem(itemStart, u.Kind, p.nameOf(isImportableName)))
		}
	}
	p.endStatement()
	u.Span = p.span(start)
	return u
}

func (p *parser) useItem(start uint32, kind token.Kind, name *ast.Name) *ast.UseItem {
	item := &ast.UseItem{Kind: kind, Name: name}
	if p.accept(token.As) {
		item.Alias = p.identifier()
	}
	item.Span = p.span(start)
	return item
}
