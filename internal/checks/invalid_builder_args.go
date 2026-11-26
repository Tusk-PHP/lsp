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
		if method, ok := frameworklaravel.BuilderMethodNamed(syntax.text(m.name)); ok && !method.MayNameAlias {
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

func (r *InvalidBuilderArgRule) checkCall(syntax *fileSyntax, call memberReference, method frameworklaravel.BuilderMethod, file *parser.FileNode, joins *builderJoins) []Finding {
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
		if len(args) > 1 && method.RelatedModelColumn {
			if column, ok := syntax.stringLiteral(args[1].Value); ok && onNameLine(args[1].Value) {
				startCol, endCol := syntax.stringContent(args[1].Value)
				add(r.validateAggregateSecondArg(relation, column, lineNum, startCol, endCol, owner, file))
			}
		}
		startCol, endCol := syntax.stringContent(first)
		add(r.validateArg(method, relation, at, startCol, endCol, file, joins))
	}
	if array, ok := first.(*ast.Array); ok && onNameLine(array) {
		for _, item := range array.Items {
			name := arrayItemName(item, method)
			if name == nil {
				continue
			}
			if value, ok := syntax.stringLiteral(name); ok {
				startCol, endCol := syntax.stringContent(name)
				add(r.validateArg(method, frameworklaravel.WithoutAlias(value), at, startCol, endCol, file, joins))
			}
		}
	}
	return findings
}

func arrayItemName(item *ast.ArrayItem, method frameworklaravel.BuilderMethod) ast.Expr {
	switch {
	case item == nil:
		return nil
	case item.Key != nil && method.NamesKeys():
		return item.Key
	case item.Key == nil && method.NamesValues():
		return item.Value
	}
	return nil
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
func (r *InvalidBuilderArgRule) validateAggregateSecondArg(relationName, columnName string, lineNum, startCol, endCol int, owner ast.Expr, file *parser.FileNode) *Finding {
	if !isPlainName(columnName) || relationName == "" {
		return nil
	}

	modelFQN := r.ModelResolver(file, owner).FQN
	if modelFQN == "" {
		return nil
	}

	// Resolve the related model from the relation
	relatedFQN := r.Members.RelatedModelFQN(modelFQN, frameworklaravel.WithoutAlias(relationName))
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

func (r *InvalidBuilderArgRule) validateArg(method frameworklaravel.BuilderMethod, argValue string, call builderCall, startCol, endCol int, file *parser.FileNode, joins *builderJoins) *Finding {
	if argValue == "" {
		return nil
	}

	// Strip " as alias" (e.g., "products as product_count")
	argValue = frameworklaravel.WithoutAlias(argValue)

	isRelMethod := method.Names == frameworklaravel.BuilderRelations

	// For dot-notation, validate the first segment only
	segments := strings.SplitN(argValue, ".", 2)
	checkName := segments[0]

	// Column methods with dot-notation are join-qualified — skip
	if len(segments) > 1 && !isRelMethod {
		return nil
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
	} else if method.Names == frameworklaravel.BuilderDBColumns {
		if !r.Members.IsDBColumn(modelFQN, checkName) {
			short := shortName(modelFQN)
			msg = fmt.Sprintf("Unknown column '%s' on model '%s'", checkName, short)
		}
	} else {
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

func shortName(fqn string) string {
	if idx := strings.LastIndex(fqn, "\\"); idx >= 0 {
		return fqn[idx+1:]
	}
	return fqn
}
