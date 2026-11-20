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

	// PHP 8.6, with the replacement PHP's own deprecation message names.
	"doubleval":                {"8.6", "doubleval() is deprecated since PHP 8.6, use floatval()"},
	"is_double":                {"8.6", "is_double() is deprecated since PHP 8.6, use is_float()"},
	"is_integer":               {"8.6", "is_integer() is deprecated since PHP 8.6, use is_int()"},
	"is_long":                  {"8.6", "is_long() is deprecated since PHP 8.6, use is_int()"},
	"mb_ereg":                  {"8.6", "mb_ereg() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_match":            {"8.6", "mb_ereg_match() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_replace":          {"8.6", "mb_ereg_replace() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_replace_callback": {"8.6", "mb_ereg_replace_callback() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_search":           {"8.6", "mb_ereg_search() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_search_getpos":    {"8.6", "mb_ereg_search_getpos() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_search_getregs":   {"8.6", "mb_ereg_search_getregs() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_search_init":      {"8.6", "mb_ereg_search_init() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_search_pos":       {"8.6", "mb_ereg_search_pos() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_search_regs":      {"8.6", "mb_ereg_search_regs() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_ereg_search_setpos":    {"8.6", "mb_ereg_search_setpos() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_eregi":                 {"8.6", "mb_eregi() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_eregi_replace":         {"8.6", "mb_eregi_replace() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_regex_encoding":        {"8.6", "mb_regex_encoding() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_regex_set_options":     {"8.6", "mb_regex_set_options() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"mb_split":                 {"8.6", "mb_split() is deprecated since PHP 8.6, its regex library is no longer maintained, use the preg_* functions with the u modifier"},
	"metaphone":                {"8.6", "metaphone() is deprecated since PHP 8.6, use a userland phonetic matching library"},
	"mysqli_get_charset":       {"8.6", "mysqli_get_charset() is deprecated since PHP 8.6, use mysqli_character_set_name()"},
	"mysqli_stmt_init":         {"8.6", "mysqli_stmt_init() is deprecated since PHP 8.6, use mysqli_prepare()"},
	"spl_classes":              {"8.6", "spl_classes() is deprecated since PHP 8.6, use ReflectionExtension::getClassNames()"},
	"spl_object_hash":          {"8.6", "spl_object_hash() is deprecated since PHP 8.6, use spl_object_id()"},
	"strcoll":                  {"8.6", "strcoll() is deprecated since PHP 8.6, use Collator::compare()"},
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
