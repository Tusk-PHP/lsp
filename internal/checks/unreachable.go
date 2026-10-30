package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// UnreachableCodeRule detects statements after unconditional return, throw,
// exit/die, continue, or break within the same block scope.
type UnreachableCodeRule struct{}

func (r *UnreachableCodeRule) Code() string { return "unreachable-code" }

func (r *UnreachableCodeRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	result := parser.New().Parse(source)
	if result == nil {
		return nil
	}
	tokens := make([]parser.Token, 0, len(result.Tokens))
	for _, tok := range result.Tokens {
		switch tok.Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
		default:
			tokens = append(tokens, tok)
		}
	}

	var findings []Finding
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			if m.EndLine <= m.StartLine {
				continue
			}
			findings = append(findings, findUnreachable(tokens, m.StartLine, m.EndLine)...)
		}
	}
	return findings
}

// terminalKeywords are statements that unconditionally transfer control.
var terminalKeywords = map[string]bool{
	"return": true, "throw": true, "exit": true, "die": true, "continue": true, "break": true,
}

// blockBoundaryKeywords end a run of statements without a closing brace, in
// switch cases and alternative syntax (`if (...): ... endif;`).
var blockBoundaryKeywords = map[string]bool{
	"case": true, "default": true, "endif": true, "endfor": true, "endforeach": true,
	"endwhile": true, "endswitch": true, "enddeclare": true,
}

// findUnreachable scans a method's tokens, which must exclude trivia, for code
// after terminal statements in the same block. Closures nested in the method are
// scanned the same way, since only their own statements can follow their returns.
func findUnreachable(tokens []parser.Token, startLine, endLine int) []Finding {
	var findings []Finding

	for i, tok := range tokens {
		if tok.Line < startLine {
			continue
		}
		if tok.Line > endLine {
			break
		}
		if !isTerminalKeyword(tok) || !startsStatement(tokens, i) {
			continue
		}

		end := statementEnd(tokens, i+1)
		if end < 0 || end+1 >= len(tokens) || isBlockEnd(tokens, end+1) || isLabel(tokens, end+1) {
			continue
		}
		first := tokens[end+1]
		if first.Line > endLine {
			continue
		}

		last := tokens[lastInBlock(tokens, end+1)]
		// A nested terminal can report again, so each region is reported from
		// its outermost terminal only.
		if len(findings) > 0 && !isAfter(first, findings[len(findings)-1]) {
			continue
		}
		findings = append(findings, Finding{
			StartLine: first.Line,
			StartCol:  first.Column,
			EndLine:   last.Line,
			EndCol:    last.Column + len(last.Value),
			Severity:  SeverityWarning,
			Code:      "unreachable-code",
			Message:   "Unreachable code detected",
		})
	}

	return findings
}

func isTerminalKeyword(tok parser.Token) bool {
	return (tok.Kind == parser.TokenReturn || tok.Kind == parser.TokenIdentifier) &&
		terminalKeywords[strings.ToLower(tok.Value)]
}

// startsStatement reports whether the token at i begins a statement that always
// runs when its block is reached. `if ($x) return;` and `$a ?? throw $e;` don't.
func startsStatement(tokens []parser.Token, i int) bool {
	if i == 0 {
		return true
	}
	switch tokens[i-1].Kind {
	case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace:
		return true
	case parser.TokenColon:
		return colonOpensBlock(tokens, i-1)
	}
	return false
}

// colonOpensBlock tells the colon of `case 1:` or `else:` from a ternary's, or a
// named argument's, by looking at what the statement it's part of starts with.
func colonOpensBlock(tokens []parser.Token, colon int) bool {
	depth := 0
	for i := colon - 1; i >= 0; i-- {
		switch tokens[i].Kind {
		case parser.TokenCloseParen, parser.TokenCloseBracket:
			depth++
		case parser.TokenOpenParen, parser.TokenOpenBracket:
			if depth == 0 {
				return false
			}
			depth--
		case parser.TokenQuestion:
			if depth == 0 {
				return false
			}
		case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace, parser.TokenColon:
			if depth == 0 {
				return opensBlockKeyword(tokens[i+1])
			}
		}
	}
	return opensBlockKeyword(tokens[0])
}

func opensBlockKeyword(tok parser.Token) bool {
	switch strings.ToLower(tok.Value) {
	case "case", "default", "if", "elseif", "else", "for", "foreach", "while", "switch":
		return true
	}
	return false
}

// statementEnd returns the index of the semicolon ending the statement that
// continues at start, skipping anything nested such as a closure's body or a
// match. It returns -1 if the enclosing block closes first.
func statementEnd(tokens []parser.Token, start int) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			if depth == 0 {
				return -1
			}
			depth--
		case parser.TokenSemicolon:
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// isBlockEnd reports whether the token at i ends the statements of its block.
func isBlockEnd(tokens []parser.Token, i int) bool {
	tok := tokens[i]
	if tok.Kind == parser.TokenCloseBrace || tok.Kind == parser.TokenEOF {
		return true
	}
	lower := strings.ToLower(tok.Value)
	if blockBoundaryKeywords[lower] {
		return true
	}
	// The else of alternative syntax. After a closing brace, it belongs to an
	// if statement inside the block.
	return (lower == "else" || lower == "elseif") && i > 0 && tokens[i-1].Kind != parser.TokenCloseBrace
}

// isLabel reports whether a goto label starts at i, which can be jumped to.
func isLabel(tokens []parser.Token, i int) bool {
	return tokens[i].Kind == parser.TokenIdentifier && i+1 < len(tokens) && tokens[i+1].Kind == parser.TokenColon
}

// lastInBlock returns the index of the last token before the block ends.
func lastInBlock(tokens []parser.Token, start int) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		if depth == 0 && isBlockEnd(tokens, i) {
			return i - 1
		}
		switch tokens[i].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
		}
	}
	return len(tokens) - 1
}

func isAfter(tok parser.Token, f Finding) bool {
	return tok.Line > f.EndLine || (tok.Line == f.EndLine && tok.Column >= f.EndCol)
}
