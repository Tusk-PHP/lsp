package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// deprecatedFunction is a global PHP function deprecated from a PHP version.
type deprecatedFunction struct {
	since   string // PHP major.minor that deprecated it
	message string
}

// deprecatedFunctions maps lowercase global PHP function names to when they
// were deprecated and the message reported when they are called.
var deprecatedFunctions = map[string]deprecatedFunction{
	"each":            {"7.2", "each() is deprecated since PHP 7.2"},
	"create_function": {"7.2", "create_function() is deprecated since PHP 7.2"},
	"utf8_encode":     {"8.2", "utf8_encode() is deprecated since PHP 8.2, use mb_convert_encoding()"},
	"utf8_decode":     {"8.2", "utf8_decode() is deprecated since PHP 8.2, use mb_convert_encoding()"},
}

// DeprecatedFunctionRule flags calls to deprecated global PHP functions.
// Only real global function calls count: method and static calls with the
// same name (`$collection->each(...)`, `Arr::each(...)`), declarations,
// namespace-qualified names, strings and comments are ignored.
type DeprecatedFunctionRule struct {
	// PHPVersion is the project's lowest supported PHP major.minor. A function
	// deprecated in a later version isn't reported: the project still supports
	// PHP versions where it is fine. Empty reports every deprecation.
	PHPVersion string
}

func (r *DeprecatedFunctionRule) Code() string { return "deprecated" }

func (r *DeprecatedFunctionRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	var findings []Finding
	mask := newMaskState(source)
	prevTail := "" // last non-blank code of the previous line, for chains split across lines

	for lineNum, raw := range strings.Split(source, "\n") {
		line := maskPHPLine(raw, &mask)
		for _, match := range unknownFunctionCallRe.FindAllStringSubmatchIndex(line, -1) {
			start, end := match[2], match[3]
			name := strings.TrimPrefix(line[start:end], "\\")
			dep, ok := deprecatedFunctions[strings.ToLower(name)]
			if !ok || (r.PHPVersion != "" && compareBuiltinVersionsStrings(r.PHPVersion, dep.since) < 0) {
				continue
			}
			if shouldSkipFunctionCall(line, start, name) || isMemberAccessBefore(line[:start], prevTail) {
				continue
			}
			findings = append(findings, Finding{
				StartLine: lineNum,
				StartCol:  start,
				EndLine:   lineNum,
				EndCol:    end,
				Severity:  SeverityWarning,
				Code:      "deprecated",
				Message:   dep.message,
				Tags:      []Tag{TagDeprecated},
			})
		}
		if t := strings.TrimSpace(line); t != "" {
			prevTail = t
		}
	}
	return findings
}

// isMemberAccessBefore reports whether the code right before a call name
// (ignoring whitespace, including a line break) is `->`, `?->` or `::`,
// meaning the name is a method rather than a global function.
func isMemberAccessBefore(before, prevTail string) bool {
	p := strings.TrimRight(before, " \t")
	if p == "" {
		p = prevTail
	}
	return strings.HasSuffix(p, "->") || strings.HasSuffix(p, "::")
}
