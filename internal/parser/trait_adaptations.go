package parser

import "strings"

type TraitAdaptations struct {
	Precedences []TraitPrecedence
	Aliases     []TraitAlias
}

type TraitPrecedence struct {
	Trait     string
	Method    string
	InsteadOf []string
}

type TraitAlias struct {
	Trait      string
	Method     string
	Alias      string
	Visibility string
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
	method, trait := p.readAdaptationName(), ""
	if p.peek().Kind == TokenDoubleColon {
		p.advance()
		p.skipCommentTokens()
		if !isNameToken(p.peek()) {
			return
		}
		trait, method = method, p.advance().Value
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
		rule := TraitPrecedence{Trait: trait, Method: method}
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
		rule := TraitAlias{Trait: trait, Method: method}
		p.skipCommentTokens()
		switch p.peek().Kind {
		case TokenPublic, TokenProtected, TokenPrivate:
			rule.Visibility = strings.ToLower(p.advance().Value)
			p.skipCommentTokens()
		}
		if p.peek().Kind != TokenSemicolon && isNameToken(p.peek()) {
			rule.Alias = p.advance().Value
		}
		if rule.Alias != "" || rule.Visibility != "" {
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
