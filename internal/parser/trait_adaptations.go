package parser

import "strings"

type TraitAdaptations struct {
	Precedences []TraitPrecedence
	Aliases     []TraitAlias
}

type TraitPrecedence struct {
	Trait      string
	Method     string
	InsteadOf  []string
	MethodLine int
	MethodCol  int
}

type TraitAlias struct {
	Trait      string
	Method     string
	Alias      string
	Visibility string
	Final      bool
	MethodLine int
	MethodCol  int
}

// MethodAt returns the trait and method named by the rule whose method name
// covers line and col. The trait is empty when the rule names none.
func (t TraitAdaptations) MethodAt(line, col int) (trait, method string, ok bool) {
	covers := func(methodLine, methodCol int, method string) bool {
		return methodLine == line && methodCol <= col && col <= methodCol+len(method)
	}
	for _, p := range t.Precedences {
		if covers(p.MethodLine, p.MethodCol, p.Method) {
			return p.Trait, p.Method, true
		}
	}
	for _, a := range t.Aliases {
		if covers(a.MethodLine, a.MethodCol, a.Method) {
			return a.Trait, a.Method, true
		}
	}
	return "", "", false
}

func (p *structParser) parseTraitAdaptations() {
	p.advance()
	for {
		p.skipCommentTokens()
		switch p.peek().Kind {
		case TokenEOF:
			return
		case TokenCloseBrace:
			p.advance()
			return
		}
		start := p.pos
		p.parseTraitAdaptation()
		for kind := p.peek().Kind; kind != TokenEOF && kind != TokenCloseBrace; kind = p.peek().Kind {
			p.advance()
			if kind == TokenSemicolon {
				break
			}
		}
		if p.pos == start {
			p.advance()
		}
	}
}

func (p *structParser) parseTraitAdaptation() {
	at := p.peek()
	method, trait := p.readAdaptationName(), ""
	if p.peek().Kind == TokenDoubleColon {
		p.advance()
		p.skipCommentTokens()
		if !isNameToken(p.peek()) {
			return
		}
		at = p.advance()
		trait, method = method, at.Value
	}
	if method == "" {
		return
	}
	p.skipCommentTokens()
	keyword := p.peek()
	if keyword.Kind != TokenIdentifier {
		return
	}
	switch strings.ToLower(keyword.Value) {
	case "insteadof":
		p.advance()
		rule := TraitPrecedence{Trait: trait, Method: method, MethodLine: at.Line, MethodCol: at.Column}
		for {
			p.skipCommentTokens()
			name := p.readQualifiedName()
			if name == "" {
				break
			}
			rule.InsteadOf = append(rule.InsteadOf, name)
			p.skipCommentTokens()
			if _, ok := p.expect(TokenComma); !ok {
				break
			}
		}
		if trait != "" && len(rule.InsteadOf) > 0 {
			p.traitRules.Precedences = append(p.traitRules.Precedences, rule)
		}
	case "as":
		p.advance()
		rule := TraitAlias{Trait: trait, Method: method, MethodLine: at.Line, MethodCol: at.Column}
		p.skipCommentTokens()
		switch p.peek().Kind {
		case TokenPublic, TokenProtected, TokenPrivate:
			rule.Visibility = strings.ToLower(p.advance().Value)
			p.skipCommentTokens()
		case TokenFinal:
			p.advance()
			rule.Final = true
			p.skipCommentTokens()
		}
		if p.peek().Kind != TokenSemicolon && isNameToken(p.peek()) {
			rule.Alias = p.advance().Value
		}
		if rule.Alias != "" || rule.Visibility != "" || rule.Final {
			p.traitRules.Aliases = append(p.traitRules.Aliases, rule)
		}
	}
}

func (p *structParser) readAdaptationName() string {
	if name := p.readQualifiedName(); name != "" {
		return name
	}
	if isNameToken(p.peek()) {
		return p.advance().Value
	}
	return ""
}

func (p *structParser) readQualifiedName() string {
	var b strings.Builder
	afterSeparator := true
	for {
		t := p.peek()
		switch {
		case t.Kind == TokenBackslash:
			afterSeparator = true
		case t.Kind == TokenIdentifier && afterSeparator:
			afterSeparator = false
		default:
			return b.String()
		}
		b.WriteString(t.Value)
		p.advance()
	}
}

func (p *structParser) skipCommentTokens() {
	for k := p.peek().Kind; k == TokenComment || k == TokenDocComment; k = p.peek().Kind {
		p.advance()
	}
}

// TraitRuleMethodAt returns the class-like whose trait rule has its method
// name at line and col, with the trait and method that rule names.
func (f *FileNode) TraitRuleMethodAt(line, col int) (owner, trait, method string, ok bool) {
	at := func(name string, rules TraitAdaptations) bool {
		trait, method, ok = rules.MethodAt(line, col)
		owner = name
		return ok
	}
	for _, c := range f.AnonymousClasses {
		if at(c.FullName, c.TraitRules) {
			return
		}
	}
	for _, c := range f.Classes {
		if at(c.FullName, c.TraitRules) {
			return
		}
	}
	for _, t := range f.Traits {
		if at(t.FullName, t.TraitRules) {
			return
		}
	}
	for _, e := range f.Enums {
		if at(e.FullName, e.TraitRules) {
			return
		}
	}
	return "", "", "", false
}
