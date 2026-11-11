package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// Code behind a version check is written for several versions of PHP or of a
// library, and only one of them is installed. Laravel, for example, calls
// PHPUnit's ErrorHandler::enable() with an argument on PHPUnit 12.3.4 and
// later and without one before. Rules that compare code against the installed
// signatures can't tell which branch runs, so they stay quiet in all of them.

// tokenSpan is a range of token indexes, both ends included.
type tokenSpan struct{ start, end int }

// versionGatedSpans returns the if/elseif/else chains in which any condition
// checks a version: version_compare(), the PHP_VERSION constants, a class's
// VERSION or *_VERSION constant (Symfony's Kernel::VERSION_ID), or Composer's
// InstalledVersions. Each span covers the whole chain, every branch of it.
func versionGatedSpans(tokens []parser.Token) []tokenSpan {
	var spans []tokenSpan
	for i := 0; i < len(tokens); i++ {
		if !isKeywordToken(tokens[i], "if") {
			continue
		}
		// `else if` is part of the chain that its first `if` starts.
		if p := prevSignificantTokenIdx(tokens, i-1); p >= 0 && isKeywordToken(tokens[p], "else") {
			continue
		}
		end, gated := ifChainEnd(tokens, i)
		if gated && end > i {
			spans = append(spans, tokenSpan{i, end})
			i = end
		}
	}
	return spans
}

// inTokenSpans reports whether token index i lies inside one of the spans.
func inTokenSpans(spans []tokenSpan, i int) bool {
	for _, s := range spans {
		if i >= s.start && i <= s.end {
			return true
		}
	}
	return false
}

// ifChainEnd walks the chain an `if` at tokens[start] begins, through its
// elseif, else if and else branches, and returns the index of its last token
// and whether any of its conditions checks a version. It returns -1 when the
// chain is cut off, as in a file still being typed.
func ifChainEnd(tokens []parser.Token, start int) (int, bool) {
	gated := false
	for j := start; ; {
		closeIdx, versioned := conditionEnd(tokens, j)
		if closeIdx < 0 {
			return -1, gated
		}
		gated = gated || versioned
		body := nextSignificantTokenIdx(tokens, closeIdx+1)
		if body < 0 {
			return -1, gated
		}
		if tokens[body].Kind == parser.TokenColon {
			end, versioned := altSyntaxEnd(tokens, body+1)
			return end, gated || versioned
		}
		end := branchEnd(tokens, body)
		if end < 0 {
			return -1, gated
		}
		next := nextSignificantTokenIdx(tokens, end+1)
		switch {
		case next >= 0 && isKeywordToken(tokens[next], "elseif"):
			j = next
		case next >= 0 && isKeywordToken(tokens[next], "else"):
			after := nextSignificantTokenIdx(tokens, next+1)
			if after < 0 {
				return end, gated
			}
			if isKeywordToken(tokens[after], "if") {
				j = after
				continue
			}
			if elseEnd := branchEnd(tokens, after); elseEnd > end {
				return elseEnd, gated
			}
			return end, gated
		default:
			return end, gated
		}
	}
}

// conditionEnd takes the `if` or `elseif` at tokens[kw] and returns the index
// of its condition's closing parenthesis, and whether the condition checks a
// version.
func conditionEnd(tokens []parser.Token, kw int) (int, bool) {
	open := nextSignificantTokenIdx(tokens, kw+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return -1, false
	}
	closeIdx := findMatchingTokenIdx(tokens, open, parser.TokenOpenParen, parser.TokenCloseParen)
	if closeIdx < 0 {
		return -1, false
	}
	return closeIdx, isVersionCheck(tokens[open+1 : closeIdx])
}

// branchEnd returns the index of the last token of the branch body starting
// at tokens[start]: the matching `}` of a block, or the `;` ending a single
// statement.
func branchEnd(tokens []parser.Token, start int) int {
	if tokens[start].Kind == parser.TokenOpenBrace {
		return findMatchingTokenIdx(tokens, start, parser.TokenOpenBrace, parser.TokenCloseBrace)
	}
	return statementEnd(tokens, start)
}

// altSyntaxEnd finds the `endif` that closes an `if (…):` whose body starts at
// tokens[start], skipping nested alternative-syntax ifs, and reports whether an
// `elseif (…):` of the same chain checks a version.
func altSyntaxEnd(tokens []parser.Token, start int) (int, bool) {
	depth, gated := 0, false
	for k := start; k < len(tokens); k++ {
		tok := tokens[k]
		switch {
		case isKeywordToken(tok, "if"), isKeywordToken(tok, "elseif"):
			closeIdx, versioned := conditionEnd(tokens, k)
			if closeIdx < 0 {
				continue
			}
			colon := nextSignificantTokenIdx(tokens, closeIdx+1)
			if colon < 0 || tokens[colon].Kind != parser.TokenColon {
				continue
			}
			if isKeywordToken(tok, "if") {
				depth++
			} else if depth == 0 {
				gated = gated || versioned
			}
			k = colon
		case isKeywordToken(tok, "endif"):
			if depth == 0 {
				return k, gated
			}
			depth--
		}
	}
	return -1, gated
}

// phpVersionConstants are the predefined constants that hold PHP's version.
var phpVersionConstants = map[string]bool{
	"PHP_VERSION": true, "PHP_VERSION_ID": true, "PHP_MAJOR_VERSION": true,
	"PHP_MINOR_VERSION": true, "PHP_RELEASE_VERSION": true,
}

// isVersionCheck reports whether a condition's tokens check a version.
func isVersionCheck(cond []parser.Token) bool {
	for k, tok := range cond {
		if tok.Kind != parser.TokenIdentifier {
			continue
		}
		name := strings.TrimPrefix(tok.Value, "\\")
		if i := strings.LastIndexByte(name, '\\'); i >= 0 {
			name = name[i+1:]
		}
		switch {
		case strings.EqualFold(name, "version_compare"), name == "InstalledVersions":
			return true
		case phpVersionConstants[name]:
			return true
		}
		// A class constant such as Kernel::VERSION_ID or Foo::MAJOR_VERSION.
		if p := prevSignificantTokenIdx(cond, k-1); p >= 0 && cond[p].Kind == parser.TokenDoubleColon {
			if name == "VERSION" || name == "VERSION_ID" || strings.HasSuffix(name, "_VERSION") || strings.HasSuffix(name, "_VERSION_ID") {
				return true
			}
		}
	}
	return false
}

// isKeywordToken reports whether tok is the keyword kw, in any case.
func isKeywordToken(tok parser.Token, kw string) bool {
	return tok.Kind == parser.TokenIdentifier && strings.EqualFold(tok.Value, kw)
}
