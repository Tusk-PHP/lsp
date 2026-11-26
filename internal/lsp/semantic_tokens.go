package lsp

import (
	"cmp"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/document"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// Semantic token type indices (must match the advertised legend order)
const (
	semTokNamespace     = 0
	semTokClass         = 1
	semTokEnum          = 2
	semTokInterface     = 3
	semTokTypeParameter = 4
	semTokParameter     = 5
	semTokVariable      = 6
	semTokProperty      = 7
	semTokEnumMember    = 8
	semTokFunction      = 9
	semTokMethod        = 10
	semTokKeyword       = 11
	semTokString        = 12
	semTokNumber        = 13
	semTokComment       = 14
	semTokOperator      = 15
	semTokDecorator     = 16
)

// Semantic token modifier bit flags
const (
	semModDeclaration = 1 << 0
	semModStatic      = 1 << 1
	semModReadonly    = 1 << 2
	semModDeprecated  = 1 << 3
)

// SemanticTokensLegend is the legend advertised in ServerCapabilities.
var SemanticTokensLegend = protocol.SemanticTokensLegend{
	TokenTypes: []string{
		"namespace", "class", "enum", "interface", "typeParameter",
		"parameter", "variable", "property", "enumMember", "function",
		"method", "keyword", "string", "number", "comment", "operator", "decorator",
	},
	TokenModifiers: []string{"declaration", "static", "readonly", "deprecated"},
}

func lexedSemanticType(kind token.Kind) (uint32, bool) {
	switch kind {
	case token.ConstantString, token.DoubleQuote, token.StartHeredoc, token.Backtick:
		return semTokString, true
	case token.IntegerLiteral, token.FloatLiteral:
		return semTokNumber, true
	case token.Comment, token.DocComment:
		return semTokComment, true
	case token.Variable:
		return semTokVariable, true
	case token.Public, token.Protected, token.Private, token.PublicSet, token.ProtectedSet, token.PrivateSet,
		token.Static, token.Abstract, token.Final, token.Readonly, token.Return, token.New,
		token.Class, token.Interface, token.Trait, token.Enum, token.Function, token.Fn, token.Namespace,
		token.Use, token.Extends, token.Implements, token.Const, token.Var:
		return semTokKeyword, true
	}
	return 0, false
}

type semToken struct {
	line      int
	startChar int
	length    int
	tokenType uint32
	modifiers uint32
}

var declaredSemanticTypes = [...]uint32{
	declaredClass:     semTokClass,
	declaredInterface: semTokInterface,
	declaredTrait:     semTokClass,
	declaredEnum:      semTokEnum,
	declaredEnumCase:  semTokEnumMember,
	declaredMethod:    semTokMethod,
	declaredFunction:  semTokFunction,
	declaredProperty:  semTokProperty,
	declaredParameter: semTokParameter,
}

func (s *Server) handleSemanticTokensFull(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.SemanticTokensParams](msg)
	if !ok {
		return
	}

	source := s.getDocument(params.TextDocument.URI)
	data := buildSemanticTokens(source, document.NewColumns(source, s.encoding))
	s.sendResponse(msg.ID, protocol.SemanticTokens{Data: data})
}

func buildSemanticTokens(source string, cols *document.Columns) []uint32 {
	lines := ast.NewLines(source)
	var tokens []semToken
	file := parser.ParseSeeingTokens(source, token.Latest, func(all []lexer.Token) {
		tokens = lexedSemanticTokens(source, lines, all)
	})
	for _, d := range declarationsOf(source, file) {
		mods := uint32(semModDeclaration)
		if d.static {
			mods |= semModStatic
		}
		tokens = appendSemToken(tokens, lines, d.name, declaredSemanticTypes[d.kind], mods)
	}

	slices.SortFunc(tokens, func(a, b semToken) int {
		return cmp.Or(cmp.Compare(a.line, b.line), cmp.Compare(a.startChar, b.startChar))
	})

	deduped := tokens[:0]
	prevLine, prevEnd := -1, -1
	for _, tok := range tokens {
		if tok.length <= 0 {
			continue
		}
		if tok.line == prevLine && tok.startChar < prevEnd {
			last := &deduped[len(deduped)-1]
			if last.startChar == tok.startChar && tok.modifiers&semModDeclaration != 0 && last.modifiers&semModDeclaration == 0 {
				*last = tok
			}
			continue
		}
		deduped = append(deduped, tok)
		prevLine, prevEnd = tok.line, tok.startChar+tok.length
	}

	result := make([]uint32, 0, len(deduped)*5)
	prevEncLine := 0
	prevEncStart := 0
	for _, tok := range deduped {
		start := cols.ToClient(tok.line, tok.startChar)
		length := cols.ToClient(tok.line, tok.startChar+tok.length) - start
		deltaLine := tok.line - prevEncLine
		deltaStart := start
		if deltaLine == 0 {
			deltaStart = start - prevEncStart
		}
		result = append(result,
			uint32(deltaLine),
			uint32(deltaStart),
			uint32(length),
			tok.tokenType,
			tok.modifiers,
		)
		prevEncLine = tok.line
		prevEncStart = start
	}

	return result
}

func appendSemToken(out []semToken, lines ast.Lines, span ast.Span, tokenType, modifiers uint32) []semToken {
	line, col := lines.Position(span.Start)
	return append(out, semToken{line: line, startChar: col, length: int(span.End - span.Start), tokenType: tokenType, modifiers: modifiers})
}

func lexedSemanticTokens(source string, lines ast.Lines, all []lexer.Token) []semToken {
	out := make([]semToken, 0, len(all)/4)
	prev := token.Invalid
	for i := 0; i < len(all); i++ {
		tok := all[i]
		if tok.Kind == token.Whitespace {
			continue
		}
		span := ast.Span{Start: tok.Start, End: tok.End}
		if lexer.OpensLiteral(tok.Kind) {
			i = lexer.LiteralEnd(all, i)
			span.End = all[i].End
		}
		if tokenType, ok := lexedSemanticType(tok.Kind); ok {
			out = appendSemToken(out, lines, span, tokenType, 0)
		}
		switch {
		case prev == token.Namespace && isDeclaredNamespace(tok.Kind):
			span.Start += uint32(strings.LastIndexByte(tok.Text(source), '\\') + 1)
			out = appendSemToken(out, lines, span, semTokNamespace, semModDeclaration)
		case prev == token.Attribute && isAttributeName(tok.Kind):
			out = appendSemToken(out, lines, span, semTokDecorator, 0)
		}
		if !tok.Kind.IsTrivia() {
			prev = tok.Kind
		}
	}
	return out
}

func isDeclaredNamespace(kind token.Kind) bool {
	return kind == token.Identifier || kind == token.NameQualified
}

func isAttributeName(kind token.Kind) bool {
	return kind == token.Identifier || kind == token.NameQualified || kind == token.NameFullyQualified
}
