package checks

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// AuthNames reads the Gate ability or middleware name a call argument holds,
// and types the model an ability is checked against.
type AuthNames interface {
	AuthName(file *parser.FileNode, arg ast.CallArg) (frameworklaravel.AuthName, bool)
	ModelClass(file *parser.FileNode, e ast.Expr) string
}

// UnknownAuthNameRule reports a Gate ability or middleware name, written as
// a plain string, that the Laravel app surely doesn't define. It reports
// nothing while the app registers names it can't read.
type UnknownAuthNameRule struct {
	Authorization *frameworklaravel.Authorization
	Names         AuthNames
}

func (r *UnknownAuthNameRule) Codes() []rule.Code {
	return []rule.Code{rule.UnknownAbility, rule.UnknownMiddleware}
}

func (r *UnknownAuthNameRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || file.Tree == nil {
		return nil
	}
	c := authNameCheck{rule: r, file: file, index: index, lines: ast.NewLines(source)}
	ast.Walk(file.Tree, func(n ast.Node) bool {
		var args *ast.ArgList
		switch n := n.(type) {
		case *ast.MethodCall:
			args = n.Args
		case *ast.StaticCall:
			args = n.Args
		case *ast.New:
			args = n.Args
		default:
			return true
		}
		if args != nil {
			c.call(n.(ast.Expr), args)
		}
		return true
	})
	return c.findings
}

type authNameCheck struct {
	rule     *UnknownAuthNameRule
	file     *parser.FileNode
	index    *symbols.Index
	lines    ast.Lines
	findings []Finding
}

func (c *authNameCheck) call(call ast.Expr, args *ast.ArgList) {
	for _, arg := range args.Args {
		if arg.Name != nil || arg.Unpack {
			continue
		}
		if lit, ok := plainString(arg.Value); ok {
			c.literal(call, lit)
			continue
		}
		array, ok := arg.Value.(*ast.Array)
		if !ok {
			continue
		}
		for _, item := range array.Items {
			if item == nil || item.Key != nil || item.Unpack {
				continue
			}
			if lit, ok := plainString(item.Value); ok {
				c.literal(call, lit)
			}
		}
	}
}

func plainString(e ast.Expr) (*ast.Literal, bool) {
	lit, ok := e.(*ast.Literal)
	return lit, ok && lit.Kind == token.ConstantString && lit.End-lit.Start > 2
}

func (c *authNameCheck) literal(call ast.Expr, lit *ast.Literal) {
	text := c.file.Source[lit.Start+1 : lit.End-1]
	if strings.ContainsAny(text, `\$`) {
		return
	}
	arg, ok := ast.CallArgOf(c.file.Tree, c.file.Source, call, lit.Start+1)
	if !ok {
		return
	}
	name, ok := c.rule.Names.AuthName(c.file, arg)
	if !ok {
		return
	}
	auth := c.rule.Authorization
	start := lit.Start + 1
	if name.Kind == frameworklaravel.MiddlewareName {
		if auth.UnknownMiddleware(name.Name) {
			c.report(rule.UnknownMiddleware, start, len(name.Name), "Middleware `"+name.Name+"` is not registered")
			return
		}
		ability, at, model, isCan := frameworklaravel.CanMiddleware(text)
		if isCan && auth.UnknownAbility(ability, modelParam(model)) {
			c.report(rule.UnknownAbility, start+uint32(at), len(ability), "Gate ability `"+ability+"` is not defined")
		}
		return
	}
	if auth.UnknownAbility(name.Name, c.modelArg(name.Model)) {
		c.report(rule.UnknownAbility, start, len(name.Name), "Gate ability `"+name.Name+"` is not defined")
	}
}

func (c *authNameCheck) modelArg(e ast.Expr) frameworklaravel.ModelArg {
	if e == nil || ast.IsMissing(e) {
		return frameworklaravel.ModelArg{}
	}
	return frameworklaravel.ModelArg{Given: true, Class: c.knownClass(c.rule.Names.ModelClass(c.file, e))}
}

func modelParam(param string) frameworklaravel.ModelArg {
	return frameworklaravel.ModelArg{Given: param != ""}
}

func (c *authNameCheck) knownClass(class string) string {
	class = strings.TrimPrefix(class, `\`)
	if class == "" || c.index.Lookup(class) == nil {
		return ""
	}
	return class
}

func (c *authNameCheck) report(code rule.Code, start uint32, length int, message string) {
	line, col := c.lines.Position(start)
	c.findings = append(c.findings, Finding{
		StartLine: line, StartCol: col, EndLine: line, EndCol: col + length,
		Severity: rule.SeverityWarning, Code: code, Message: message,
	})
}
