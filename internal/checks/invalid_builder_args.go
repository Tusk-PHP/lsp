package checks

import (
	"fmt"
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/scope"
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

// ModelResolverFunc gives the Eloquent model a builder call is made on, from
// the call's receiver node in file: the class of `Post::where(`, or the
// builder or model `$query->where(` is called on. Its FQN is "" when the
// model can't be worked out.
type ModelResolverFunc func(file *parser.FileNode, receiver ast.Expr) frameworklaravel.QueriedModel

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
	Shared        *Shared
}

func (r *InvalidBuilderArgRule) Codes() []rule.Code {
	return []rule.Code{rule.UnknownColumn, rule.UnknownRelation}
}

func (r *InvalidBuilderArgRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if r.ModelResolver == nil || r.Members == nil || file == nil {
		return nil
	}
	syntax := r.Shared.tree(source, file)
	var findings []Finding
	joins := &builderJoins{source: source}
	for _, m := range syntax.references().members {
		if !m.call() || len(m.args.Args) == 0 {
			continue
		}
		method := syntax.text(m.name)
		if builderRelationMethods[method] || builderColumnMethods[method] || builderDBColumnMethods[method] {
			findings = append(findings, r.checkCall(syntax, m, method, file, joins)...)
		}
	}
	return findings
}

type builderJoins struct {
	source string
	tokens []parser.Token
	doc    *scope.Document
}

func (j *builderJoins) mayBeJoined(line, col int) bool {
	if j.tokens == nil {
		j.tokens = parser.Tokenize(j.source)
		j.doc = scope.Collect(j.source)
	}
	return frameworklaravel.BuilderMayBeJoined(j.tokens, j.doc, line, col)
}

func (r *InvalidBuilderArgRule) checkCall(syntax *fileSyntax, call memberReference, method string, file *parser.FileNode, joins *builderJoins) []Finding {
	lineNum := syntax.line(call.name)
	owner := call.owner
	at := builderCall{line: lineNum, owner: owner}
	at.operatorLine, at.operatorCol = syntax.lines.Position(syntax.memberOperator(call.name))
	onNameLine := func(n ast.Node) bool {
		start, _ := syntax.lines.Position(n.Bounds().Start)
		end, _ := syntax.lines.Position(n.Bounds().End)
		return start == lineNum && end == lineNum
	}
	var findings []Finding
	add := func(f *Finding) {
		if f != nil {
			findings = append(findings, *f)
		}
	}
	args := call.args.Args
	first := args[0].Value
	if relation, ok := syntax.stringLiteral(first); ok && onNameLine(first) {
		if len(args) > 1 && aggregateRelationMethods[method] {
			if column, ok := syntax.stringLiteral(args[1].Value); ok && onNameLine(args[1].Value) {
				startCol, endCol := syntax.stringContent(args[1].Value)
				add(r.validateAggregateSecondArg(method, relation, column, lineNum, startCol, endCol, owner, file))
			}
		}
		startCol, endCol := syntax.stringContent(first)
		add(r.validateArg(method, relation, at, startCol, endCol, file, joins))
	}
	if array, ok := first.(*ast.Array); ok && onNameLine(array) {
		for _, item := range array.Items {
			name := item.Key
			if name == nil {
				name = item.Value
			}
			if value, ok := syntax.stringLiteral(name); ok {
				startCol, endCol := syntax.stringContent(name)
				add(r.validateArg(method, stripAlias(value), at, startCol, endCol, file, joins))
			}
		}
	}
	return findings
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

// validateAggregateSecondArg validates the second argument of aggregate methods
// like withSum('relation', 'column') — the column must exist on the related model.
func (r *InvalidBuilderArgRule) validateAggregateSecondArg(method, relationName, columnName string, lineNum, startCol, endCol int, owner ast.Expr, file *parser.FileNode) *Finding {
	if !isPlainName(columnName) || relationName == "" {
		return nil
	}

	modelFQN := r.ModelResolver(file, owner).FQN
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
			Severity:  rule.SeverityWarning,
			Code:      rule.UnknownColumn,
			Message:   fmt.Sprintf("Unknown column '%s' on related model '%s'", columnName, shortName(relatedFQN)),
		}
	}
	return nil
}

type builderCall struct {
	line         int
	operatorLine int
	operatorCol  int
	owner        ast.Expr
}

func (r *InvalidBuilderArgRule) validateArg(method, argValue string, call builderCall, startCol, endCol int, file *parser.FileNode, joins *builderJoins) *Finding {
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

	model := r.ModelResolver(file, call.owner)
	modelFQN := model.FQN
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
	if !isRelMethod && (model.OtherTables || joins.mayBeJoined(call.operatorLine, call.operatorCol)) {
		return nil
	}

	code := rule.UnknownColumn
	if isRelMethod {
		code = rule.UnknownRelation
	}

	return &Finding{
		StartLine: call.line,
		StartCol:  startCol,
		EndLine:   call.line,
		EndCol:    endCol,
		Severity:  rule.SeverityWarning,
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
