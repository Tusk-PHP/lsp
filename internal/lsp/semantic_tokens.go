package lsp

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/document"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
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

var lexedSemanticTypes = func() map[token.Kind]uint32 {
	types := map[token.Kind]uint32{
		token.ConstantString: semTokString, token.DoubleQuote: semTokString, token.StartHeredoc: semTokString, token.Backtick: semTokString,
		token.IntegerLiteral: semTokNumber, token.FloatLiteral: semTokNumber,
		token.Comment: semTokComment, token.DocComment: semTokComment,
		token.Variable: semTokVariable,
	}
	for _, kind := range []token.Kind{
		token.Public, token.Protected, token.Private, token.PublicSet, token.ProtectedSet, token.PrivateSet,
		token.Static, token.Abstract, token.Final, token.Readonly, token.Return, token.New,
		token.Class, token.Interface, token.Trait, token.Enum, token.Function, token.Fn, token.Namespace,
		token.Use, token.Extends, token.Implements, token.Const, token.Var,
	} {
		types[kind] = semTokKeyword
	}
	return types
}()

type semToken struct {
	line      int
	startChar int
	length    int
	tokenType uint32
	modifiers uint32
}

func (s *Server) handleSemanticTokensFull(msg *jsonRPCMessage) {
	params, ok := s.decodeParams[protocol.SemanticTokensParams](msg)
	if !ok {
		return
	}

	uri := params.TextDocument.URI
	source := s.getDocument(uri)
	pr := parser.New().Parse(source)

	data := buildSemanticTokens(pr, source, document.NewColumns(source, s.encoding))
	s.sendResponse(msg.ID, protocol.SemanticTokens{Data: data})
}

// buildSemanticTokens builds the LSP relative-encoded semantic token data from a ParseResult.
func buildSemanticTokens(pr *parser.ParseResult, source string, cols *document.Columns) []uint32 {
	var tokens []semToken

	// Track ranges already covered by declaration tokens to avoid duplicates.
	type coveredKey struct {
		line, start, end int
	}
	covered := make(map[coveredKey]bool)

	tokens = append(tokens, lexedSemanticTokens(source)...)

	// -- Walk declarations for typed tokens --

	// Helper: find column of identifier after a keyword on a given line
	findIdentCol := func(line int, after string) int {
		if line >= len(pr.Lines) {
			return 0
		}
		ln := pr.Lines[line]
		idx := strings.Index(ln, after)
		if idx < 0 {
			return 0
		}
		// Skip past the keyword
		pos := idx + len(after)
		// Skip whitespace
		for pos < len(ln) && (ln[pos] == ' ' || ln[pos] == '\t') {
			pos++
		}
		return pos
	}

	// Classes
	for _, cls := range pr.Classes {
		col := findIdentCol(cls.Line, "class")
		t := semToken{
			line:      cls.Line,
			startChar: col,
			length:    len(cls.Name),
			tokenType: semTokClass,
			modifiers: semModDeclaration,
		}
		covered[coveredKey{t.line, t.startChar, t.startChar + t.length}] = true
		tokens = append(tokens, t)

		for _, m := range cls.Methods {
			mods := uint32(semModDeclaration)
			if m.IsStatic {
				mods |= semModStatic
			}
			col := findIdentCol(m.Line, "function")
			mt := semToken{
				line:      m.Line,
				startChar: col,
				length:    len(m.Name),
				tokenType: semTokMethod,
				modifiers: mods,
			}
			covered[coveredKey{mt.line, mt.startChar, mt.startChar + mt.length}] = true
			tokens = append(tokens, mt)
		}

		for _, prop := range cls.Properties {
			col := findPropCol(pr, prop.Line, prop.Name)
			if col >= 0 {
				pt := semToken{
					line:      prop.Line,
					startChar: col,
					length:    len(prop.Name),
					tokenType: semTokProperty,
					modifiers: semModDeclaration,
				}
				covered[coveredKey{pt.line, pt.startChar, pt.startChar + pt.length}] = true
				tokens = append(tokens, pt)
			}
		}
	}

	// Interfaces
	for _, iface := range pr.Interfaces {
		col := findIdentCol(iface.Line, "interface")
		t := semToken{
			line:      iface.Line,
			startChar: col,
			length:    len(iface.Name),
			tokenType: semTokInterface,
			modifiers: semModDeclaration,
		}
		covered[coveredKey{t.line, t.startChar, t.startChar + t.length}] = true
		tokens = append(tokens, t)

		for _, m := range iface.Methods {
			col := findIdentCol(m.Line, "function")
			mt := semToken{
				line:      m.Line,
				startChar: col,
				length:    len(m.Name),
				tokenType: semTokMethod,
				modifiers: semModDeclaration,
			}
			covered[coveredKey{mt.line, mt.startChar, mt.startChar + mt.length}] = true
			tokens = append(tokens, mt)
		}
	}

	// Traits
	for _, trait := range pr.Traits {
		col := findIdentCol(trait.Line, "trait")
		t := semToken{
			line:      trait.Line,
			startChar: col,
			length:    len(trait.Name),
			tokenType: semTokClass, // Traits use class token type
			modifiers: semModDeclaration,
		}
		covered[coveredKey{t.line, t.startChar, t.startChar + t.length}] = true
		tokens = append(tokens, t)

		for _, m := range trait.Methods {
			mods := uint32(semModDeclaration)
			if m.IsStatic {
				mods |= semModStatic
			}
			col := findIdentCol(m.Line, "function")
			mt := semToken{
				line:      m.Line,
				startChar: col,
				length:    len(m.Name),
				tokenType: semTokMethod,
				modifiers: mods,
			}
			covered[coveredKey{mt.line, mt.startChar, mt.startChar + mt.length}] = true
			tokens = append(tokens, mt)
		}
	}

	// Enums
	for _, e := range pr.Enums {
		col := findIdentCol(e.Line, "enum")
		t := semToken{
			line:      e.Line,
			startChar: col,
			length:    len(e.Name),
			tokenType: semTokEnum,
			modifiers: semModDeclaration,
		}
		covered[coveredKey{t.line, t.startChar, t.startChar + t.length}] = true
		tokens = append(tokens, t)

		for _, c := range e.Cases {
			col := findIdentCol(c.Line, "case")
			ct := semToken{
				line:      c.Line,
				startChar: col,
				length:    len(c.Name),
				tokenType: semTokEnumMember,
				modifiers: semModDeclaration,
			}
			covered[coveredKey{ct.line, ct.startChar, ct.startChar + ct.length}] = true
			tokens = append(tokens, ct)
		}

		for _, m := range e.Methods {
			col := findIdentCol(m.Line, "function")
			mt := semToken{
				line:      m.Line,
				startChar: col,
				length:    len(m.Name),
				tokenType: semTokMethod,
				modifiers: semModDeclaration,
			}
			covered[coveredKey{mt.line, mt.startChar, mt.startChar + mt.length}] = true
			tokens = append(tokens, mt)
		}
	}

	// Functions
	for _, fn := range pr.Functions {
		col := findIdentCol(fn.Line, "function")
		ft := semToken{
			line:      fn.Line,
			startChar: col,
			length:    len(fn.Name),
			tokenType: semTokFunction,
			modifiers: semModDeclaration,
		}
		covered[coveredKey{ft.line, ft.startChar, ft.startChar + ft.length}] = true
		tokens = append(tokens, ft)

		for _, param := range fn.Params {
			name := param.Name
			// Find param on function line or nearby
			col := findParamCol(pr, fn.Line, fn.EndLine, name)
			if col >= 0 {
				pt := semToken{
					line:      fn.Line,
					startChar: col,
					length:    len(name),
					tokenType: semTokParameter,
					modifiers: semModDeclaration,
				}
				covered[coveredKey{pt.line, pt.startChar, pt.startChar + pt.length}] = true
				tokens = append(tokens, pt)
			}
		}
	}

	// Sort by (line, startChar) ascending
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].line != tokens[j].line {
			return tokens[i].line < tokens[j].line
		}
		return tokens[i].startChar < tokens[j].startChar
	})

	// Deduplicate: skip tokens that overlap with the previous token on the same line
	deduped := make([]semToken, 0, len(tokens))
	prevLine := -1
	prevEnd := -1
	for _, tok := range tokens {
		if tok.length <= 0 {
			continue
		}
		end := tok.startChar + tok.length
		if tok.line == prevLine && tok.startChar < prevEnd {
			// Overlapping token on same line — skip duplicates, prefer declaration tokens
			// Remove the last added if the new one is a declaration and the old isn't
			if len(deduped) > 0 {
				last := deduped[len(deduped)-1]
				if last.line == tok.line && last.startChar == tok.startChar {
					// Same start: prefer declaration
					if tok.modifiers&semModDeclaration != 0 && last.modifiers&semModDeclaration == 0 {
						deduped[len(deduped)-1] = tok
					}
				}
			}
			continue
		}
		deduped = append(deduped, tok)
		prevLine = tok.line
		prevEnd = end
	}

	// Encode with relative delta encoding
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

func lexedSemanticTokens(source string) []semToken {
	lines := ast.NewLines(source)
	all := lexer.Lex(source, token.Latest)
	add := func(out []semToken, tok lexer.Token, tokenType, modifiers uint32) []semToken {
		line, col := lines.Position(tok.Start)
		return append(out, semToken{line: line, startChar: col, length: int(tok.End - tok.Start), tokenType: tokenType, modifiers: modifiers})
	}
	out := make([]semToken, 0, len(all)/4)
	prev := token.Invalid
	for i := 0; i < len(all); i++ {
		tok := all[i]
		if tok.Kind == token.Whitespace {
			continue
		}
		if lexer.OpensLiteral(tok.Kind) {
			i = lexer.LiteralEnd(all, i)
			tok.End = all[i].End
		}
		if tokenType, ok := lexedSemanticTypes[tok.Kind]; ok {
			out = add(out, tok, tokenType, 0)
		}
		switch {
		case prev == token.Namespace && isDeclaredNamespace(tok.Kind):
			tok.Start += uint32(strings.LastIndexByte(tok.Text(source), '\\') + 1)
			out = add(out, tok, semTokNamespace, semModDeclaration)
		case prev == token.Attribute && isAttributeName(tok.Kind):
			out = add(out, tok, semTokDecorator, 0)
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

// findPropCol finds the column of a property name (including the $ sigil) on the given line.
func findPropCol(pr *parser.ParseResult, line int, name string) int {
	if line >= len(pr.Lines) {
		return -1
	}
	ln := pr.Lines[line]
	// Property names include $ in the source
	search := "$" + strings.TrimPrefix(name, "$")
	idx := strings.Index(ln, search)
	if idx < 0 {
		return -1
	}
	return idx
}

// findParamCol finds the column of a parameter name in the function signature lines.
func findParamCol(pr *parser.ParseResult, startLine, endLine int, name string) int {
	search := "$" + strings.TrimPrefix(name, "$")
	for l := startLine; l <= startLine && l < len(pr.Lines); l++ {
		ln := pr.Lines[l]
		idx := strings.Index(ln, search)
		if idx >= 0 {
			return idx
		}
	}
	return -1
}
