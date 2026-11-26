package parser

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func Interpolates(lit string) bool {
	if rest, ok := strings.CutPrefix(lit, "<<<"); ok {
		return !strings.HasPrefix(strings.TrimLeft(rest, " \t"), "'")
	}
	return len(lit) >= 2 && lit[0] == '"'
}

func InterpolatedTokens(lit Token) []Token {
	if lit.Kind != TokenStringLiteral || !Interpolates(lit.Value) || strings.IndexByte(lit.Value, '$') < 0 {
		return nil
	}
	parts := lexer.LexCode(lit.Value, token.Latest)
	p := placer{lit: lit, line: lit.Line, column: lit.Column, capacity: len(parts)}
	var tokens []Token
	depth, codeStart, inOffset := 0, 0, false
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if depth > 0 {
			switch part.Kind {
			case token.LeftBrace, token.CurlyOpen, token.DollarOpenCurlyBraces:
				depth++
			case token.RightBrace:
				depth--
				if depth == 0 {
					tokens = p.appendCode(tokens, parts[codeStart:i])
				}
			}
			continue
		}
		switch part.Kind {
		case token.CurlyOpen:
			depth, codeStart = 1, i+1
		case token.DollarOpenCurlyBraces:
			depth, codeStart = 1, i+1
			if i+1 < len(parts) && parts[i+1].Kind == token.StringVarname {
				i++
				codeStart = i + 1
			}
		case token.LeftBracket:
			inOffset = true
		case token.RightBracket:
			inOffset = false
		case token.Variable:
			end := i + 1
			if !inOffset && i+2 < len(parts) && isObjectOperator(parts[i+1].Kind) && parts[i+2].Kind == token.Identifier {
				end = i + 3
			}
			tokens = p.appendCode(tokens, parts[i:end])
			i = end - 1
		}
	}
	if depth > 0 {
		tokens = p.appendCode(tokens, parts[codeStart:])
	}
	return tokens
}

func isObjectOperator(kind token.Kind) bool {
	return kind == token.ObjectOperator || kind == token.NullsafeObjectOperator
}

type placer struct {
	lit          Token
	capacity     int
	offset       int
	line, column int
}

func (p *placer) at(offset int) Token {
	skipped := p.lit.Value[p.offset:offset]
	if nl := strings.LastIndexByte(skipped, '\n'); nl >= 0 {
		p.line += strings.Count(skipped, "\n")
		p.column = len(skipped) - nl - 1
	} else {
		p.column += len(skipped)
	}
	p.offset = offset
	return Token{Line: p.line, Column: p.column, Offset: p.lit.Offset + offset}
}

var directKinds = map[token.Kind]TokenKind{
	token.Whitespace:             TokenUnknown,
	token.Variable:               TokenVariable,
	token.Identifier:             TokenIdentifier,
	token.ObjectOperator:         TokenArrow,
	token.NullsafeObjectOperator: TokenArrow,
	token.LeftBracket:            TokenOpenBracket,
	token.RightBracket:           TokenCloseBracket,
}

func (p *placer) appendCode(tokens []Token, parts []lexer.Token) []Token {
	if len(parts) == 0 {
		return tokens
	}
	if tokens == nil {
		tokens = make([]Token, 0, p.capacity)
	}
	for _, part := range parts {
		if _, ok := directKinds[part.Kind]; !ok {
			return p.appendTokenized(tokens, int(parts[0].Start), int(parts[len(parts)-1].End))
		}
	}
	for _, part := range parts {
		if part.Kind == token.Whitespace {
			continue
		}
		tok := p.at(int(part.Start))
		tok.Kind, tok.Value = directKinds[part.Kind], part.Text(p.lit.Value)
		switch part.Kind {
		case token.Identifier:
			tok.Kind = identifierToKind(tok.Value)
		case token.NullsafeObjectOperator:
			tok.Kind, tok.Value = TokenQuestion, "?"
			tokens = append(tokens, tok)
			tok = p.at(int(part.Start) + 1)
			tok.Kind, tok.Value = TokenArrow, "->"
		}
		tokens = append(tokens, tok)
	}
	return tokens
}

const codePrefix = "<?php "

func (p *placer) appendTokenized(tokens []Token, start, end int) []Token {
	first := p.at(start)
	code, _ := tokenize(codePrefix + p.lit.Value[start:end])
	for _, tok := range code {
		if tok.Kind == TokenPHP || tok.Kind == TokenEOF {
			continue
		}
		if tok.Line == 0 {
			tok.Column += first.Column - len(codePrefix)
		}
		tok.Line += first.Line
		tok.Offset += first.Offset - len(codePrefix)
		tokens = append(tokens, tok)
	}
	return tokens
}
