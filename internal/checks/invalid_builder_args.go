package checks

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Builder method categories — mirrors the lists in completion/columns.go.
var builderColumnMethods = map[string]bool{
	"where": true, "whereIn": true, "whereNotIn": true,
	"whereNull": true, "whereNotNull": true,
	"whereBetween": true, "whereNotBetween": true,
	"whereDate": true, "whereMonth": true, "whereDay": true,
	"whereYear": true, "whereTime": true, "whereColumn": true,
	"orderBy": true, "orderByDesc": true,
	"latest": true, "oldest": true,
	"groupBy": true, "having": true,
	"select": true, "addSelect": true,
	"pluck": true, "value": true,
	"increment": true, "decrement": true,
}

var builderDBColumnMethods = map[string]bool{
	"get": true,
}

var builderRelationMethods = map[string]bool{
	"with": true, "without": true,
	"has": true, "doesntHave": true,
	"whereHas": true, "whereDoesntHave": true,
	"withCount": true, "withSum": true, "withAvg": true,
	"withMin": true, "withMax": true, "withExists": true,
	"load": true, "loadMissing": true, "loadCount": true,
}

// ModelResolverFunc resolves the Eloquent model FQN from a builder chain
// expression. prefix is the line up to and including the call's opening
// parenthesis (`Post::query()->where(`). Returns "" if the model cannot be
// determined.
type ModelResolverFunc func(prefix string, method string, source string, line int, file *parser.FileNode) string

// MemberChecker provides methods to verify columns and relations on a model.
type MemberChecker interface {
	IsColumn(modelFQN string, name string) bool
	IsDBColumn(modelFQN string, name string) bool
	IsRelation(modelFQN string, name string) bool
	// RelatedModelFQN returns the FQN of the model that a relation points to.
	// Returns "" if the related model cannot be determined.
	RelatedModelFQN(modelFQN string, relationName string) string
	// KnowsColumns reports whether the model's column list is complete, so a
	// name missing from it is really not a column. A list built from
	// migrations or docblocks can miss columns and is not complete.
	KnowsColumns(modelFQN string) bool
	// KnowsRelations reports whether every class and trait the model is made
	// of is indexed, so a relation method missing from it really is missing.
	KnowsRelations(modelFQN string) bool
}

// aliasColumnMethods take a name that may be an alias from the select list
// (`selectRaw('count(*) as total')->orderBy('total')`, `withCount('posts')`
// then `having('posts_count', ...)`) rather than a column, which the rule
// can't tell apart, so their arguments are not checked.
var aliasColumnMethods = map[string]bool{
	"orderBy": true, "orderByDesc": true, "latest": true, "oldest": true,
	"groupBy": true, "having": true,
}

// Aggregate relation methods where the second arg is a column on the related model.
var aggregateRelationMethods = map[string]bool{
	"withSum": true, "withAvg": true, "withMin": true, "withMax": true,
}

// InvalidBuilderArgRule detects string arguments in Builder method calls that
// reference columns or relations that don't exist on the resolved model.
type InvalidBuilderArgRule struct {
	ModelResolver ModelResolverFunc
	Members       MemberChecker
}

func (r *InvalidBuilderArgRule) Code() string { return "invalid-builder-arg" }

func (r *InvalidBuilderArgRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if r.ModelResolver == nil || r.Members == nil || file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding

	mask := newMaskState(source)
	for i, line := range lines {
		code := maskPHPLine(line, &mask)
		if !strings.Contains(code, "->") && !strings.Contains(code, "::") {
			continue
		}
		findings = append(findings, r.checkLine(line, code, i, source, file)...)
	}
	return findings
}

// callPrefix returns the line up to and including the opening parenthesis of
// the call a match starts at (its `->` or `::`), or false when that operator
// is inside a comment or string rather than code.
func callPrefix(line, code string, match []int) (string, bool) {
	start, nameEnd := match[0], match[3]
	if code[start:start+2] != line[start:start+2] {
		return "", false
	}
	open := strings.IndexByte(line[nameEnd:], '(')
	if open < 0 {
		return "", false
	}
	return line[:nameEnd+open+1], true
}

// isPlainName reports whether s is a bare column or relation name. Anything
// else (`*`, `count(*)`, `options->locale`, `raw sql`) is left alone.
func isPlainName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

// Matches ->method('arg') or ::method('arg') with a completed single-quoted string.
var directArgSingleRe = regexp.MustCompile(`(?:->|::)(\w+)\s*\(\s*'([^']*)'`)

// Matches ->method("arg") or ::method("arg") with a completed double-quoted string.
var directArgDoubleRe = regexp.MustCompile(`(?:->|::)(\w+)\s*\(\s*"([^"]*)"`)

// Matches string literals inside array arguments: ['val1', 'val2']
var arrayArgRe = regexp.MustCompile(`(?:->|::)(\w+)\s*\(\s*\[([^\]]*)\]`)

// Matches individual single-quoted strings.
var singleQuotedRe = regexp.MustCompile(`'([^']*)'`)

// Matches individual double-quoted strings.
var doubleQuotedRe = regexp.MustCompile(`"([^"]*)"`)

// Matches two-arg pattern: ->withSum('relation', 'column') — captures method, first and second args.
var twoArgSingleRe = regexp.MustCompile(`(?:->|::)(\w+)\s*\(\s*'([^']*)'\s*,\s*'([^']*)'`)
var twoArgDoubleRe = regexp.MustCompile(`(?:->|::)(\w+)\s*\(\s*"([^"]*)"\s*,\s*"([^"]*)"`)

func (r *InvalidBuilderArgRule) checkLine(line, code string, lineNum int, source string, file *parser.FileNode) []Finding {
	var findings []Finding

	// Check aggregate two-arg methods first: ->withSum('relation', 'column')
	for _, re := range []*regexp.Regexp{twoArgSingleRe, twoArgDoubleRe} {
		for _, match := range re.FindAllStringSubmatchIndex(line, -1) {
			method := line[match[2]:match[3]]
			if !aggregateRelationMethods[method] {
				continue
			}
			prefix, ok := callPrefix(line, code, match)
			if !ok {
				continue
			}
			firstArg := line[match[4]:match[5]]
			secondArg := line[match[6]:match[7]]
			secondStart := match[6]
			secondEnd := match[7]

			if f := r.validateAggregateSecondArg(method, firstArg, secondArg, lineNum, secondStart, secondEnd, prefix, source, file); f != nil {
				findings = append(findings, *f)
			}
		}
	}

	// Check direct string arguments: ->where('column_name', or ->where("column_name",
	for _, re := range []*regexp.Regexp{directArgSingleRe, directArgDoubleRe} {
		for _, match := range re.FindAllStringSubmatchIndex(line, -1) {
			method := line[match[2]:match[3]]
			argValue := line[match[4]:match[5]]
			argStart := match[4]
			argEnd := match[5]
			prefix, ok := callPrefix(line, code, match)
			if !ok {
				continue
			}

			if f := r.validateArg(method, argValue, lineNum, argStart, argEnd, prefix, source, file); f != nil {
				findings = append(findings, *f)
			}
		}
	}

	// Check array arguments: ->with(['products', 'category'])
	for _, match := range arrayArgRe.FindAllStringSubmatchIndex(line, -1) {
		method := line[match[2]:match[3]]
		arrayContent := line[match[4]:match[5]]
		prefix, ok := callPrefix(line, code, match)
		if !ok {
			continue
		}

		// Extract each quoted string from the array
		for _, re := range []*regexp.Regexp{singleQuotedRe, doubleQuotedRe} {
			for _, strMatch := range re.FindAllStringSubmatchIndex(arrayContent, -1) {
				argValue := arrayContent[strMatch[2]:strMatch[3]]
				absStart := match[4] + strMatch[2]
				absEnd := match[4] + strMatch[3]

				cleaned := stripAlias(argValue)
				if f := r.validateArg(method, cleaned, lineNum, absStart, absEnd, prefix, source, file); f != nil {
					findings = append(findings, *f)
				}
			}
		}
	}

	return findings
}

// validateAggregateSecondArg validates the second argument of aggregate methods
// like withSum('relation', 'column') — the column must exist on the related model.
func (r *InvalidBuilderArgRule) validateAggregateSecondArg(method, relationName, columnName string, lineNum, startCol, endCol int, prefix, source string, file *parser.FileNode) *Finding {
	if !isPlainName(columnName) || relationName == "" {
		return nil
	}

	modelFQN := r.ModelResolver(prefix, method, source, lineNum, file)
	if modelFQN == "" {
		return nil
	}

	// Resolve the related model from the relation
	relatedFQN := r.Members.RelatedModelFQN(modelFQN, stripAlias(relationName))
	if relatedFQN == "" {
		return nil // can't determine related model — skip
	}

	if r.Members.KnowsColumns(relatedFQN) && !r.Members.IsColumn(relatedFQN, columnName) {
		return &Finding{
			StartLine: lineNum,
			StartCol:  startCol,
			EndLine:   lineNum,
			EndCol:    endCol,
			Severity:  SeverityWarning,
			Code:      "unknown-column",
			Message:   fmt.Sprintf("Unknown column '%s' on related model '%s'", columnName, shortName(relatedFQN)),
		}
	}
	return nil
}

func (r *InvalidBuilderArgRule) validateArg(method, argValue string, lineNum, startCol, endCol int, prefix, source string, file *parser.FileNode) *Finding {
	if argValue == "" {
		return nil
	}

	// Strip " as alias" (e.g., "products as product_count")
	argValue = stripAlias(argValue)

	// For dot-notation in column methods (e.g., orderBy('relation.column')),
	// skip validation — it requires join context we don't have.
	isRelMethod := builderRelationMethods[method]
	isColMethod := builderColumnMethods[method]
	isDBColMethod := builderDBColumnMethods[method]

	if !isRelMethod && !isColMethod && !isDBColMethod || aliasColumnMethods[method] {
		return nil
	}

	// For dot-notation, validate the first segment only
	segments := strings.SplitN(argValue, ".", 2)
	checkName := segments[0]

	// Column methods with dot-notation are join-qualified — skip
	if len(segments) > 1 && !isRelMethod {
		return nil
	}

	// Handle closure-based array syntax: 'products' => function($q) {...}
	// The regex captures 'products' before '=>', strip anything after =>
	if idx := strings.Index(checkName, " =>"); idx >= 0 {
		checkName = checkName[:idx]
	}
	// `with('posts:id,title')` loads only those columns of the relation.
	if isRelMethod {
		if idx := strings.IndexByte(checkName, ':'); idx >= 0 {
			checkName = checkName[:idx]
		}
	}
	checkName = strings.TrimSpace(checkName)
	if !isPlainName(checkName) {
		return nil
	}

	// Resolve the model from the line up to the method call
	modelFQN := r.ModelResolver(prefix, method, source, lineNum, file)
	if modelFQN == "" {
		return nil
	}
	if isRelMethod && !r.Members.KnowsRelations(modelFQN) || !isRelMethod && !r.Members.KnowsColumns(modelFQN) {
		return nil
	}

	var msg string
	if isRelMethod {
		if !r.Members.IsRelation(modelFQN, checkName) {
			short := shortName(modelFQN)
			msg = fmt.Sprintf("Unknown relation '%s' on model '%s'", checkName, short)
		}
	} else if isDBColMethod {
		if !r.Members.IsDBColumn(modelFQN, checkName) {
			short := shortName(modelFQN)
			msg = fmt.Sprintf("Unknown column '%s' on model '%s'", checkName, short)
		}
	} else if isColMethod {
		if !r.Members.IsColumn(modelFQN, checkName) {
			short := shortName(modelFQN)
			msg = fmt.Sprintf("Unknown column '%s' on model '%s'", checkName, short)
		}
	}

	if msg == "" {
		return nil
	}

	code := "unknown-column"
	if isRelMethod {
		code = "unknown-relation"
	}

	return &Finding{
		StartLine: lineNum,
		StartCol:  startCol,
		EndLine:   lineNum,
		EndCol:    endCol,
		Severity:  SeverityWarning,
		Code:      code,
		Message:   msg,
	}
}

// stripAlias removes " as alias" from a string (e.g., "products as cnt" → "products").
func stripAlias(s string) string {
	if idx := strings.Index(s, " as "); idx >= 0 {
		return s[:idx]
	}
	return s
}

func shortName(fqn string) string {
	if idx := strings.LastIndex(fqn, "\\"); idx >= 0 {
		return fqn[idx+1:]
	}
	return fqn
}
