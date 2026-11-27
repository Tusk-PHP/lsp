package laravel

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

const (
	formRequest       = `Illuminate\Foundation\Http\FormRequest`
	httpRequest       = `Illuminate\Http\Request`
	validatesRequests = `Illuminate\Foundation\Validation\ValidatesRequests`
)

// ValidationSyntax is what finding the validation rules of a file needs to
// know about the names and expressions in it.
type ValidationSyntax struct {
	// ClassName returns the class a name written in the file stands for.
	ClassName func(name string) string
	// ExprClass returns the class of what the expression src[start:end]
	// evaluates to, or "" when it can't tell.
	ExprClass func(start, end uint32) string
	// IsSubtype reports whether child is parent or extends, implements or
	// uses it.
	IsSubtype func(child, parent string) bool
	// Helper returns what a method of a class or trait supplies as rules
	// when its returned value is used as validation rules elsewhere; nil
	// follows no helper.
	Helper func(class, method string) (RulesForm, bool)
}

// RuleString is a string literal Laravel reads as validation rules: a value
// of the rules array, or an item of one of its values in array form.
type RuleString struct {
	// Start and End delimit its content, between the quotes.
	Start, End uint32
	// Piped says whether `|` separates rules in it. Laravel splits a value
	// of the rules array on every `|`, a regex's too; an item of a value in
	// array form is one rule.
	Piped bool
}

// RuleSegment is one rule in a rule string: its name runs from Start to
// NameEnd, its parameters, after a `:`, to End.
type RuleSegment struct {
	Start, NameEnd, End uint32
}

// Name is the segment's rule name as written.
func (s RuleSegment) Name(src string) string {
	return src[s.Start:s.NameEnd]
}

// Segments splits the string into its rules.
func (r RuleString) Segments(src string) []RuleSegment {
	content := src[r.Start:r.End]
	n := 1
	if r.Piped {
		n += strings.Count(content, "|")
	}
	segments := make([]RuleSegment, 0, n)
	start := r.Start
	for {
		end := r.End
		if r.Piped {
			if i := strings.IndexByte(src[start:r.End], '|'); i >= 0 {
				end = start + uint32(i)
			}
		}
		nameEnd := end
		if i := strings.IndexByte(src[start:end], ':'); i >= 0 {
			nameEnd = start + uint32(i)
		}
		segments = append(segments, RuleSegment{Start: start, NameEnd: nameEnd, End: end})
		if end == r.End {
			return segments
		}
		start = end + 1
	}
}

// ValidationRuleStrings visits every rule string of a file: in the rules of
// $request->validate(), $this->validate($request, …), validateWithBag(),
// Validator::make(), Validator::validate(), validator() and the array a
// form request's rules() returns, and in what a helper method returns into
// one of those (see ValidationHelpers). A rule built at run time, by
// concatenation, interpolation or from a variable, is not visited.
func ValidationRuleStrings(src string, file *ast.File, syn ValidationSyntax, visit func(RuleString)) {
	w := ruleWalk{src: src, syn: syn, visit: visit, keep: func(ast.Span) bool { return true }}
	w.walk(file)
}

// ValidationRuleAt returns the rule string and the rule in it that hold the
// byte offset: in its name, its parameters or at either end.
func ValidationRuleAt(src string, file *ast.File, syn ValidationSyntax, offset uint32) (RuleString, RuleSegment, bool) {
	var found RuleString
	ok := false
	w := ruleWalk{
		src: src, syn: syn,
		keep: func(s ast.Span) bool { return s.Start <= offset && offset <= s.End },
		visit: func(r RuleString) {
			if r.Start <= offset && offset <= r.End {
				found, ok = r, true
			}
		},
	}
	w.walk(file)
	if !ok {
		return RuleString{}, RuleSegment{}, false
	}
	for _, seg := range found.Segments(src) {
		if seg.Start <= offset && offset <= seg.End {
			return found, seg, true
		}
	}
	return RuleString{}, RuleSegment{}, false
}

type ruleWalk struct {
	src       string
	syn       ValidationSyntax
	keep      func(ast.Span) bool
	visit     func(RuleString)
	namespace string
}

func (w *ruleWalk) walk(root ast.Node) {
	ast.Walk(root, func(n ast.Node) bool {
		if ns, ok := n.(*ast.Namespace); ok {
			w.namespace = ""
			if ns.Name != nil {
				w.namespace = ns.Name.Text(w.src)
			}
		}
		if !w.keep(n.Bounds()) {
			return false
		}
		switch n := n.(type) {
		case *ast.ClassDecl:
			read := w.formRequest(n)
			if n.Name != nil {
				w.helpers(n.Name, n.Members, read)
			}
		case *ast.TraitDecl:
			w.helpers(n.Name, n.Members, false)
		case *ast.MethodCall:
			w.rulesArg(n.Args, w.methodRulesArg(n))
		case *ast.StaticCall:
			w.rulesArg(n.Args, w.staticRulesArg(n))
		case *ast.Call:
			w.rulesArg(n.Args, w.functionRulesArg(n))
		}
		return true
	})
}

func (w *ruleWalk) formRequest(class *ast.ClassDecl) bool {
	if class.Extends == nil || !w.syn.IsSubtype(w.syn.ClassName(class.Extends.Text(w.src)), formRequest) {
		return false
	}
	for _, m := range class.Members {
		method, ok := m.(*ast.Method)
		if !ok || method.Body == nil || !strings.EqualFold(method.Name.Text(w.src), "rules") || !w.keep(method.Span) {
			continue
		}
		w.returned(method.Body, func(value ast.Expr) {
			if rules, ok := value.(*ast.Array); ok {
				w.rules(rules)
			}
		})
	}
	return true
}

func (w *ruleWalk) helpers(name *ast.Identifier, members []ast.Member, rulesRead bool) {
	if w.syn.Helper == nil {
		return
	}
	class := name.Text(w.src)
	if w.namespace != "" {
		class = w.namespace + `\` + class
	}
	for _, m := range members {
		method, ok := m.(*ast.Method)
		if !ok || method.Body == nil || !w.keep(method.Span) || (rulesRead && strings.EqualFold(method.Name.Text(w.src), "rules")) {
			continue
		}
		form, ok := w.syn.Helper(class, method.Name.Text(w.src))
		if !ok {
			continue
		}
		w.returned(method.Body, func(value ast.Expr) {
			if rules, isArray := value.(*ast.Array); isArray && form == RulesArray {
				w.rules(rules)
			} else if form == FieldRules {
				w.fieldRules(value)
			}
		})
	}
}

func (w *ruleWalk) returned(body ast.Node, visit func(ast.Expr)) {
	ast.Walk(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Closure, *ast.ArrowFunction, *ast.ClassDecl, *ast.FunctionDecl, *ast.New:
			return false
		case *ast.Return:
			if n.Value != nil {
				visit(n.Value)
			}
			return false
		}
		return w.keep(n.Bounds())
	})
}

func (w *ruleWalk) methodRulesArg(call *ast.MethodCall) int {
	name, ok := call.Name.(*ast.IdentifierExpr)
	if !ok {
		return -1
	}
	shift := 0
	switch strings.ToLower(name.Text(w.src)) {
	case "validate":
	case "validatewithbag":
		shift = 1
	default:
		return -1
	}
	if len(call.Args.Args) == 0 {
		return -1
	}
	object := call.Object.Bounds()
	class := w.syn.ExprClass(object.Start, object.End)
	switch {
	case class == "":
		return -1
	case w.syn.IsSubtype(class, httpRequest):
		return shift
	case w.syn.IsSubtype(class, validatesRequests):
		return shift + 1
	}
	return -1
}

func (w *ruleWalk) staticRulesArg(call *ast.StaticCall) int {
	class, ok := call.Class.(*ast.ConstFetch)
	name, named := call.Name.(*ast.IdentifierExpr)
	if !ok || !named {
		return -1
	}
	switch strings.ToLower(name.Text(w.src)) {
	case "make", "validate":
	default:
		return -1
	}
	fqn := w.syn.ClassName(class.Name.Text(w.src))
	if fqn != validatorFacade && fqn != "Validator" {
		return -1
	}
	return 1
}

func (w *ruleWalk) functionRulesArg(call *ast.Call) int {
	callee, ok := call.Callee.(*ast.ConstFetch)
	if !ok || !strings.EqualFold(strings.TrimPrefix(callee.Name.Text(w.src), `\`), "validator") {
		return -1
	}
	return 1
}

func (w *ruleWalk) rulesArg(args *ast.ArgList, position int) {
	if position < 0 {
		return
	}
	for i, arg := range args.Args {
		if arg.Unpack {
			return
		}
		if arg.Name != nil {
			if arg.Name.Text(w.src) != "rules" {
				continue
			}
		} else if i != position {
			continue
		}
		if rules, ok := arg.Value.(*ast.Array); ok {
			w.rules(rules)
		}
		return
	}
}

func (w *ruleWalk) rules(rules *ast.Array) {
	for _, item := range rules.Items {
		if item == nil || item.Key == nil || item.Unpack || !w.keep(item.Span) {
			continue
		}
		w.fieldRules(item.Value)
	}
}

func (w *ruleWalk) fieldRules(value ast.Expr) {
	switch value := value.(type) {
	case *ast.Literal:
		w.ruleString(value, true)
	case *ast.Array:
		for _, rule := range value.Items {
			if rule == nil || rule.Key != nil || rule.Unpack {
				continue
			}
			if lit, ok := rule.Value.(*ast.Literal); ok {
				w.ruleString(lit, false)
			}
		}
	}
}

func (w *ruleWalk) ruleString(lit *ast.Literal, piped bool) {
	if lit.Kind != token.ConstantString || lit.End-lit.Start < 2 || w.src[lit.Start] == 'b' || w.src[lit.Start] == 'B' {
		return
	}
	w.visit(RuleString{Start: lit.Start + 1, End: lit.End - 1, Piped: piped})
}
