package parser

import (
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func (p *parser) attributes() []*ast.AttributeGroup {
	var groups []*ast.AttributeGroup
	for p.peek() == token.Attribute {
		start := p.start()
		p.next()
		group := &ast.AttributeGroup{}
		for {
			attrStart := p.start()
			attr := &ast.Attribute{Name: p.name()}
			if p.peek() == token.LeftParen {
				attr.Args = p.args()
			}
			attr.Span = p.span(attrStart)
			group.Attrs = append(group.Attrs, attr)
			if !p.accept(token.Comma) || p.peek() == token.RightBracket {
				break
			}
		}
		p.expect(token.RightBracket)
		group.Span = p.span(start)
		groups = append(groups, group)
	}
	return groups
}

func isModifier(k token.Kind) bool {
	switch k {
	case token.Public, token.Protected, token.Private, token.Static, token.Abstract, token.Final, token.Readonly, token.Var,
		token.PublicSet, token.ProtectedSet, token.PrivateSet:
		return true
	}
	return false
}

func (p *parser) modifiers() []*ast.Modifier {
	var mods []*ast.Modifier
	for isModifier(p.peek()) {
		t := p.next()
		mods = append(mods, &ast.Modifier{Span: ast.Span{Start: t.Start, End: t.End}, Kind: t.Kind})
	}
	return mods
}

func (p *parser) rejectFinal(mods []*ast.Modifier, message string) {
	for _, m := range mods {
		if m.Kind == token.Final {
			p.errorAt(m.Span, message)
		}
	}
}

func (p *parser) params() []*ast.Param {
	var params []*ast.Param
	if !p.expect(token.LeftParen) {
		return nil
	}
	for p.peek() != token.RightParen && p.peek() != eof {
		params = append(params, p.param())
		if !p.accept(token.Comma) {
			break
		}
	}
	p.expect(token.RightParen)
	return params
}

func (p *parser) param() *ast.Param {
	start := p.start()
	param := &ast.Param{Doc: p.doc(), Attrs: p.attributes(), Modifiers: p.modifiers()}
	if p.version < token.PHP85 {
		p.rejectFinal(param.Modifiers, "cannot use the final modifier on a parameter")
	}
	if startsType(p.peek()) {
		param.Type = p.typeDecl()
	}
	if isAmpersand(p.peek()) {
		p.next()
		param.ByRef = true
	}
	param.Variadic = p.accept(token.Ellipsis)
	varStart := p.start()
	if p.expect(token.Variable) {
		param.Var = &ast.Variable{Span: p.span(varStart)}
	}
	if p.accept(token.Equals) {
		param.Default = p.expr()
	}
	if p.peek() == token.LeftBrace {
		param.Hooks = p.hooks()
	}
	param.Span = p.span(start)
	if param.Doc == (ast.Span{}) {
		param.Doc = p.doc()
	}
	return param
}

func startsType(k token.Kind) bool {
	switch k {
	case token.Question, token.LeftParen, token.Identifier, token.NameQualified, token.NameFullyQualified, token.NameRelative,
		token.Array, token.Callable, token.Static:
		return true
	}
	return false
}

func (p *parser) returnType() ast.Type {
	if !p.accept(token.Colon) {
		return nil
	}
	return p.typeDecl()
}

func (p *parser) typeDecl() ast.Type {
	start := p.start()
	if p.accept(token.Question) {
		return &ast.NullableType{Inner: p.namedType(), Span: p.span(start)}
	}
	first := p.typeAtom()
	switch {
	case p.peek() == token.BitwiseOr:
		union := &ast.UnionType{Types: []ast.Type{first}}
		for p.accept(token.BitwiseOr) {
			union.Types = append(union.Types, p.typeAtom())
		}
		union.Span = p.span(start)
		return union
	case p.peek() == token.AmpersandNotFollowedByVarOrVararg:
		if _, grouped := first.(*ast.IntersectionType); grouped {
			return first
		}
		return p.intersection(first, start)
	}
	return first
}

func (p *parser) typeAtom() ast.Type {
	start := p.start()
	if p.accept(token.LeftParen) {
		inner := p.intersection(p.namedType(), start)
		p.expect(token.RightParen)
		inner.Span = p.span(start)
		p.requires(token.PHP82, start, "disjunctive normal form types")
		return inner
	}
	return p.namedType()
}

func (p *parser) intersection(first ast.Type, start uint32) *ast.IntersectionType {
	types := []ast.Type{first}
	for p.accept(token.AmpersandNotFollowedByVarOrVararg) {
		types = append(types, p.namedType())
	}
	return &ast.IntersectionType{Types: types, Span: p.span(start)}
}

func (p *parser) namedType() ast.Type {
	start := p.start()
	switch k := p.peek(); k {
	case token.Array, token.Callable, token.Static:
		p.next()
		return &ast.NamedType{Name: &ast.Name{Span: p.span(start), Kind: k}, Span: p.span(start)}
	}
	return &ast.NamedType{Name: p.name(), Span: p.span(start)}
}

func (p *parser) names() []*ast.Name {
	names := []*ast.Name{p.name()}
	for p.accept(token.Comma) {
		names = append(names, p.name())
	}
	return names
}

func (p *parser) functionDecl(start uint32, doc ast.Span, attrs []*ast.AttributeGroup) ast.Stmt {
	p.next()
	fn := &ast.FunctionDecl{Attrs: attrs, Doc: doc}
	if isAmpersand(p.peek()) {
		p.next()
		fn.ByRef = true
	}
	if p.peek() == token.Readonly {
		t := p.next()
		fn.Name = &ast.Identifier{Span: ast.Span{Start: t.Start, End: t.End}}
	} else {
		fn.Name = p.identifier()
	}
	fn.Params = p.params()
	fn.ReturnType = p.returnType()
	fn.Body = p.block()
	fn.Span = p.span(start)
	return fn
}

func (p *parser) classDecl(start uint32, doc ast.Span, attrs []*ast.AttributeGroup) ast.Stmt {
	c := &ast.ClassDecl{Attrs: attrs, Doc: doc}
	for k := p.peek(); k == token.Abstract || k == token.Final || k == token.Readonly; k = p.peek() {
		t := p.next()
		if k == token.Readonly {
			p.requires(token.PHP82, t.Start, "readonly classes")
		}
		c.Modifiers = append(c.Modifiers, &ast.Modifier{Span: ast.Span{Start: t.Start, End: t.End}, Kind: k})
	}
	p.expect(token.Class)
	c.Name = p.identifier()
	p.classTail(c)
	c.Span = p.span(start)
	return c
}

func (p *parser) classTail(c *ast.ClassDecl) {
	if p.accept(token.Extends) {
		c.Extends = p.name()
	}
	if p.accept(token.Implements) {
		c.Implements = p.names()
	}
	c.Members = p.members()
}

func (p *parser) anonymousClass() *ast.ClassDecl {
	start := p.start()
	c := &ast.ClassDecl{Attrs: p.attributes()}
	if p.peek() == token.Readonly {
		t := p.next()
		p.requires(token.PHP83, t.Start, "readonly anonymous classes")
		c.Modifiers = append(c.Modifiers, &ast.Modifier{Span: ast.Span{Start: t.Start, End: t.End}, Kind: token.Readonly})
	}
	p.expect(token.Class)
	if p.peek() == token.LeftParen {
		c.Args = p.args()
	}
	p.classTail(c)
	c.Span = p.span(start)
	return c
}

func (p *parser) interfaceDecl(start uint32, doc ast.Span, attrs []*ast.AttributeGroup) ast.Stmt {
	p.next()
	i := &ast.InterfaceDecl{Attrs: attrs, Doc: doc, Name: p.identifier()}
	if p.accept(token.Extends) {
		i.Extends = p.names()
	}
	i.Members = p.members()
	i.Span = p.span(start)
	return i
}

func (p *parser) traitDecl(start uint32, doc ast.Span, attrs []*ast.AttributeGroup) ast.Stmt {
	p.next()
	t := &ast.TraitDecl{Attrs: attrs, Doc: doc, Name: p.identifier()}
	t.Members = p.members()
	t.Span = p.span(start)
	return t
}

func (p *parser) enumDecl(start uint32, doc ast.Span, attrs []*ast.AttributeGroup) ast.Stmt {
	p.next()
	e := &ast.EnumDecl{Attrs: attrs, Doc: doc, Name: p.identifier()}
	if p.accept(token.Colon) {
		e.BackingType = p.typeDecl()
	}
	if p.accept(token.Implements) {
		e.Implements = p.names()
	}
	e.Members = p.members()
	e.Span = p.span(start)
	return e
}

func (p *parser) members() []ast.Member {
	if !p.expect(token.LeftBrace) {
		return nil
	}
	var members []ast.Member
	for p.peek() != token.RightBrace && p.peek() != eof {
		before := p.pos
		members = append(members, p.member())
		p.ensureProgress(before)
	}
	p.expect(token.RightBrace)
	return members
}

func (p *parser) member() ast.Member {
	start, doc := p.start(), p.doc()
	attrs := p.attributes()
	switch p.peek() {
	case token.Use:
		return p.traitUse(start, doc)
	case token.Case:
		p.next()
		c := &ast.EnumCase{Attrs: attrs, Doc: doc, Name: p.memberName()}
		if p.accept(token.Equals) {
			c.Value = p.expr()
		}
		p.expect(token.Semicolon)
		c.Span = p.span(start)
		return c
	}
	mods := p.modifiers()
	switch p.peek() {
	case token.Const:
		return p.classConst(start, doc, attrs, mods)
	case token.Function:
		return p.method(start, doc, attrs, mods)
	}
	if startsType(p.peek()) || p.peek() == token.Variable {
		return p.property(start, doc, attrs, mods)
	}
	p.unexpected("expected a class member")
	p.skipTo(func(k token.Kind) bool { return k == token.Semicolon })
	p.accept(token.Semicolon)
	return &ast.BadMember{Span: p.span(start)}
}

func (p *parser) classConst(start uint32, doc ast.Span, attrs []*ast.AttributeGroup, mods []*ast.Modifier) ast.Member {
	p.next()
	c := &ast.ClassConst{Attrs: attrs, Doc: doc, Modifiers: mods}
	if p.peekAt(1) != token.Equals {
		typeStart := p.start()
		c.Type = p.typeDecl()
		p.requires(token.PHP83, typeStart, "typed class constants")
	}
	c.Items = p.constItems(p.memberName)
	p.expect(token.Semicolon)
	c.Span = p.span(start)
	return c
}

func (p *parser) constItems(name func() *ast.Identifier) []*ast.ConstItem {
	var items []*ast.ConstItem
	for {
		itemStart := p.start()
		item := &ast.ConstItem{Name: name()}
		p.expect(token.Equals)
		item.Value = p.expr()
		item.Span = p.span(itemStart)
		items = append(items, item)
		if !p.accept(token.Comma) {
			return items
		}
	}
}

func (p *parser) method(start uint32, doc ast.Span, attrs []*ast.AttributeGroup, mods []*ast.Modifier) ast.Member {
	p.next()
	m := &ast.Method{Attrs: attrs, Doc: doc, Modifiers: mods}
	if isAmpersand(p.peek()) {
		p.next()
		m.ByRef = true
	}
	m.Name = p.memberName()
	m.Params = p.params()
	m.ReturnType = p.returnType()
	if !p.accept(token.Semicolon) {
		m.Body = p.block()
	}
	m.Span = p.span(start)
	return m
}

func (p *parser) property(start uint32, doc ast.Span, attrs []*ast.AttributeGroup, mods []*ast.Modifier) ast.Member {
	if p.version == token.PHP83 {
		p.rejectFinal(mods, "cannot use the final modifier on a property")
	}
	prop := &ast.Property{Attrs: attrs, Doc: doc, Modifiers: mods}
	if p.peek() != token.Variable {
		prop.Type = p.typeDecl()
	}
	for {
		itemStart := p.start()
		item := &ast.PropertyItem{}
		if p.expect(token.Variable) {
			item.Var = &ast.Variable{Span: p.span(itemStart)}
		}
		if p.accept(token.Equals) {
			item.Default = p.expr()
		}
		item.Span = p.span(itemStart)
		prop.Items = append(prop.Items, item)
		if !p.accept(token.Comma) {
			break
		}
	}
	if p.peek() == token.LeftBrace {
		prop.Hooks = p.hooks()
	} else {
		p.expect(token.Semicolon)
	}
	prop.Span = p.span(start)
	return prop
}

func (p *parser) hooks() []*ast.PropertyHook {
	start := p.start()
	p.next()
	p.requires(token.PHP84, start, "property hooks")
	var hooks []*ast.PropertyHook
	for p.peek() != token.RightBrace && p.peek() != eof {
		hookStart := p.start()
		hook := &ast.PropertyHook{Attrs: p.attributes(), Modifiers: p.modifiers()}
		if isAmpersand(p.peek()) {
			p.next()
			hook.ByRef = true
		}
		hook.Name = p.identifier()
		if p.peek() == token.LeftParen {
			hook.Params = p.params()
		}
		switch {
		case p.accept(token.DoubleArrow):
			hook.Body = p.expr()
			p.expect(token.Semicolon)
		case p.peek() == token.LeftBrace:
			if body := p.block(); body != nil {
				hook.Body = body
			}
		default:
			p.expect(token.Semicolon)
		}
		hook.Span = p.span(hookStart)
		hooks = append(hooks, hook)
		if hook.Name == nil {
			break
		}
	}
	p.expect(token.RightBrace)
	return hooks
}

func (p *parser) traitUse(start uint32, doc ast.Span) ast.Member {
	p.next()
	u := &ast.TraitUse{Doc: doc, Traits: p.names()}
	if p.accept(token.LeftBrace) {
		for p.peek() != token.RightBrace && p.peek() != eof {
			before := p.pos
			u.Adaptations = append(u.Adaptations, p.adaptation())
			p.ensureProgress(before)
		}
		p.expect(token.RightBrace)
	} else {
		p.expect(token.Semicolon)
	}
	u.Span = p.span(start)
	return u
}

func (p *parser) adaptation() *ast.TraitAdaptation {
	start := p.start()
	a := &ast.TraitAdaptation{}
	if p.peekAt(1) == token.DoubleColon {
		a.Trait = p.name()
		p.next()
	}
	a.Method = p.memberName()
	switch {
	case p.accept(token.InsteadOf):
		a.InsteadOf = p.names()
	case p.expect(token.As):
		if k := p.peek(); k == token.Public || k == token.Protected || k == token.Private {
			t := p.next()
			a.Visibility = &ast.Modifier{Span: ast.Span{Start: t.Start, End: t.End}, Kind: k}
		}
		if a.Visibility == nil || isSemiReserved(p.peek()) {
			a.Alias = p.memberName()
		}
	}
	p.expect(token.Semicolon)
	a.Span = p.span(start)
	return a
}
