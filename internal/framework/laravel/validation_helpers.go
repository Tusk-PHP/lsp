package laravel

import (
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// RulesForm is what a method that supplies validation rules returns.
type RulesForm uint8

const (
	// FieldRules are one field's rules: a piped string or a list of rules.
	FieldRules RulesForm = iota + 1
	// RulesArray is a rules array, from field names to their rules.
	RulesArray
)

const maxHelperDepth = 4

// ValidationHelpers are the project's methods whose returned value ends up
// as validation rules: `'password' => $this->passwordRules()`,
// `...$this->profileRules()` or `Validator::make($input, self::rules())`,
// and the methods those return their rules from in turn. Calls on $this,
// self, static and a named class are followed; each project file's are
// replaced when it is indexed again. A nil *ValidationHelpers knows none.
type ValidationHelpers struct {
	mu     sync.Mutex
	files  map[string]fileHelpers
	byName map[string]map[string]bool
}

type fileHelpers struct {
	uses  []helperUse
	edges []helperEdge
}

type helperUse struct {
	method, owner string
	form          RulesForm
	rulesClass    bool
}

type helperEdge struct {
	method, owner string
	form          RulesForm
	from          string
	fromForm      RulesForm
}

// NewValidationHelpers returns an empty set of helpers.
func NewValidationHelpers() *ValidationHelpers {
	return &ValidationHelpers{files: map[string]fileHelpers{}, byName: map[string]map[string]bool{}}
}

// IndexFile reads a project file's rule helper calls; an empty source
// removes the file's.
func (h *ValidationHelpers) IndexFile(uri, source string) {
	if h == nil {
		return
	}
	found := helpersIn(source)
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.files[uri]; ok {
		for _, name := range old.names() {
			delete(h.byName[name], uri)
			if len(h.byName[name]) == 0 {
				delete(h.byName, name)
			}
		}
		delete(h.files, uri)
	}
	if len(found.uses)+len(found.edges) == 0 {
		return
	}
	h.files[uri] = found
	for _, name := range found.names() {
		if h.byName[name] == nil {
			h.byName[name] = map[string]bool{}
		}
		h.byName[name][uri] = true
	}
}

func (f fileHelpers) names() []string {
	names := make([]string, 0, len(f.uses)+len(f.edges))
	for _, u := range f.uses {
		names = append(names, u.method)
	}
	for _, e := range f.edges {
		names = append(names, e.method)
	}
	return names
}

// Form returns what the method of class supplies as validation rules,
// when some rules position in the project takes its returned value.
func (h *ValidationHelpers) Form(index *symbols.Index, class, method string) (RulesForm, bool) {
	if h == nil || class == "" {
		return 0, false
	}
	return h.form(index, class, strings.ToLower(method), maxHelperDepth)
}

func (h *ValidationHelpers) form(index *symbols.Index, class, method string, depth int) (RulesForm, bool) {
	if depth == 0 {
		return 0, false
	}
	uses, edges := h.named(method)
	for _, u := range uses {
		if !index.IsSubtypeOf(u.owner, class) {
			continue
		}
		if u.rulesClass && !isRulesClass(index, u.owner) {
			continue
		}
		return u.form, true
	}
	for _, e := range edges {
		if !index.IsSubtypeOf(e.owner, class) {
			continue
		}
		if form, ok := h.form(index, e.owner, e.from, depth-1); ok && form == e.fromForm {
			return e.form, true
		}
	}
	return 0, false
}

func (h *ValidationHelpers) named(method string) ([]helperUse, []helperEdge) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var uses []helperUse
	var edges []helperEdge
	for uri := range h.byName[method] {
		f := h.files[uri]
		for _, u := range f.uses {
			if u.method == method {
				uses = append(uses, u)
			}
		}
		for _, e := range f.edges {
			if e.method == method {
				edges = append(edges, e)
			}
		}
	}
	return uses, edges
}

func isRulesClass(index *symbols.Index, class string) bool {
	return index.IsSubtypeOf(class, formRequest)
}

func keyed(a *ast.Array) bool {
	for _, item := range a.Items {
		if item != nil && item.Key != nil {
			return true
		}
	}
	return false
}

func helpersIn(src string) fileHelpers {
	if !strings.Contains(src, "alidat") && !strings.Contains(src, "rules") {
		return fileHelpers{}
	}
	s := helperScan{src: src, names: fileNames{uses: map[string]string{}}}
	for _, stmt := range phpparser.Parse(src, token.Latest).Stmts {
		s.statement(stmt)
	}
	return s.found
}

type helperScan struct {
	src   string
	names fileNames
	class string
	found fileHelpers
}

func (s *helperScan) statement(stmt ast.Stmt) {
	switch n := stmt.(type) {
	case *ast.Namespace:
		s.names.enter(s.src, n)
		for _, inner := range n.Stmts {
			s.statement(inner)
		}
	case *ast.Use:
		s.names.use(s.src, n)
	case *ast.ClassDecl:
		if n.Name != nil {
			s.members(n.Name.Text(s.src), n.Members)
		}
	case *ast.TraitDecl:
		s.members(n.Name.Text(s.src), n.Members)
	default:
		s.calls(stmt)
	}
}

func (s *helperScan) members(name string, members []ast.Member) {
	s.class = name
	if s.names.namespace != "" {
		s.class = s.names.namespace + `\` + name
	}
	defer func() { s.class = "" }()
	for _, m := range members {
		switch m := m.(type) {
		case *ast.Method:
			if m.Body == nil {
				continue
			}
			s.calls(m.Body)
			method := strings.ToLower(m.Name.Text(s.src))
			s.returns(m.Body, func(value ast.Expr) {
				if method == "rules" {
					s.rulesValue(value, RulesArray, true)
				}
				s.returned(method, value)
			})
		case *ast.Property:
			for _, item := range m.Items {
				if rules, ok := item.Default.(*ast.Array); ok && item.Var.Text(s.src) == "$rules" {
					s.rulesValue(rules, RulesArray, true)
				}
			}
		}
	}
}

func (s *helperScan) returns(body ast.Node, visit func(ast.Expr)) {
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
		return true
	})
}

func (s *helperScan) returned(from string, value ast.Expr) {
	edge := func(method, owner string, form, fromForm RulesForm) {
		s.found.edges = append(s.found.edges, helperEdge{method: method, owner: owner, form: form, from: from, fromForm: fromForm})
	}
	if method, owner, ok := s.helperCall(value); ok {
		edge(method, owner, RulesArray, RulesArray)
		edge(method, owner, FieldRules, FieldRules)
		return
	}
	rules, ok := value.(*ast.Array)
	if !ok {
		return
	}
	for _, item := range rules.Items {
		if item == nil {
			continue
		}
		method, owner, ok := s.helperCall(item.Value)
		switch {
		case !ok:
		case item.Unpack && keyed(rules):
			edge(method, owner, RulesArray, RulesArray)
		case item.Unpack:
			edge(method, owner, FieldRules, FieldRules)
		case item.Key != nil:
			edge(method, owner, FieldRules, RulesArray)
		}
	}
}

func (s *helperScan) rulesValue(value ast.Expr, form RulesForm, rulesClass bool) {
	use := func(method, owner string, form RulesForm) {
		s.found.uses = append(s.found.uses, helperUse{method: method, owner: owner, form: form, rulesClass: rulesClass})
	}
	if method, owner, ok := s.helperCall(value); ok {
		use(method, owner, form)
		return
	}
	rules, ok := value.(*ast.Array)
	if !ok || form != RulesArray {
		return
	}
	for _, item := range rules.Items {
		if item == nil {
			continue
		}
		method, owner, ok := s.helperCall(item.Value)
		switch {
		case !ok:
		case item.Unpack:
			use(method, owner, RulesArray)
		case item.Key != nil:
			use(method, owner, FieldRules)
		}
	}
}

func (s *helperScan) calls(root ast.Node) {
	ast.Walk(root, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Closure, *ast.ArrowFunction:
			return true
		case *ast.ClassDecl, *ast.FunctionDecl:
			return false
		case *ast.MethodCall:
			s.rulesArg(n.Args, s.methodRulesArg(n))
		case *ast.StaticCall:
			if class, ok := n.Class.(*ast.ConstFetch); ok {
				switch strings.ToLower(memberName(s.src, n.Name)) {
				case "make", "validate":
					if fqn := s.names.resolve(class.Name.Text(s.src)); fqn == validatorFacade || fqn == "Validator" {
						s.rulesArg(n.Args, 1)
					}
				}
			}
		case *ast.Call:
			if callee, ok := n.Callee.(*ast.ConstFetch); ok && strings.EqualFold(strings.TrimPrefix(callee.Name.Text(s.src), `\`), "validator") {
				s.rulesArg(n.Args, 1)
			}
		}
		return true
	})
}

func (s *helperScan) methodRulesArg(call *ast.MethodCall) int {
	switch strings.ToLower(memberName(s.src, call.Name)) {
	case "validate":
		return s.controllerShift(call)
	case "validatewithbag":
		return 1 + s.controllerShift(call)
	case "validateonly":
		return 1
	}
	return -1
}

func (s *helperScan) controllerShift(call *ast.MethodCall) int {
	object, ok := call.Object.(*ast.Variable)
	if !ok || object.Text(s.src) != "$this" || len(call.Args.Args) < 2 {
		return 0
	}
	if _, isVar := call.Args.Args[0].Value.(*ast.Variable); isVar {
		return 1
	}
	return 0
}

func (s *helperScan) rulesArg(args *ast.ArgList, position int) {
	if position < 0 {
		return
	}
	for i, arg := range args.Args {
		if arg.Unpack {
			return
		}
		if (arg.Name != nil && arg.Name.Text(s.src) == "rules") || (arg.Name == nil && i == position) {
			s.rulesValue(arg.Value, RulesArray, false)
			return
		}
	}
}

func (s *helperScan) helperCall(e ast.Expr) (method, owner string, ok bool) {
	switch call := e.(type) {
	case *ast.MethodCall:
		object, isVar := call.Object.(*ast.Variable)
		name := memberName(s.src, call.Name)
		if !isVar || object.Text(s.src) != "$this" || s.class == "" || name == "" {
			return "", "", false
		}
		return strings.ToLower(name), s.class, true
	case *ast.StaticCall:
		class, isName := call.Class.(*ast.ConstFetch)
		name := memberName(s.src, call.Name)
		if !isName || name == "" {
			return "", "", false
		}
		switch written := class.Name.Text(s.src); strings.ToLower(written) {
		case "self", "static":
			if s.class == "" {
				return "", "", false
			}
			return strings.ToLower(name), s.class, true
		case "parent":
			return "", "", false
		default:
			return strings.ToLower(name), s.names.resolve(written), true
		}
	}
	return "", "", false
}
