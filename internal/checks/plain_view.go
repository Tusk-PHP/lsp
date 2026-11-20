package checks

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
)

// isPlainView reports whether a .php file is a view: it writes HTML or other
// text outside <?php … ?> and declares no class, interface, trait or enum.
// Such a file is rendered by other code (Laravel's console components,
// Symfony's .html.php templates, WordPress themes) through include, often
// after extract(), so the variables its top level reads come from there.
//
// It reads the tokens the rule already has: the tokenizer emits an open tag
// as TokenPHP and a close tag as a `?>` semicolon, and nothing for the text
// between them.
func isPlainView(file *parser.FileNode, source string, tokens []parser.Token) bool {
	if file != nil && len(file.Classes)+len(file.Interfaces)+len(file.Traits)+len(file.Enums) > 0 {
		return false
	}
	if !parser.HasOpenTag(source) {
		return false // a snippet with no open tag is PHP throughout
	}
	htmlFrom := 0 // start of the current stretch of text outside PHP; -1 inside PHP
	for _, tok := range tokens {
		switch {
		case tok.Kind == parser.TokenPHP && htmlFrom >= 0:
			if hasText(source[htmlFrom:tok.Offset]) {
				return true
			}
			htmlFrom = -1
		case tok.Kind == parser.TokenSemicolon && tok.Value == "?>":
			htmlFrom = tok.Offset + 2
		}
	}
	return htmlFrom >= 0 && htmlFrom <= len(source) && hasText(source[htmlFrom:])
}

// hasText reports whether s has anything but whitespace. PHP drops a single
// newline right after `?>`, and whitespace alone is usually a file's
// trailing newline, so neither makes a file a view.
func hasText(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\r', '\n':
		default:
			return true
		}
	}
	return false
}
