package laravel

import (
	"slices"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

const validatorFacade = `Illuminate\Support\Facades\Validator`

// ValidationExtensions are the custom validation rules each indexed file
// registers with the validator: Validator::extend(), extendImplicit() and
// extendDependent(), and ->extend() on $validator, app('validator') or
// $this->app['validator']. Each file's are replaced when it is indexed
// again and dropped when it is removed. A nil *ValidationExtensions has
// none.
type ValidationExtensions struct {
	mu      sync.Mutex
	files   map[string]fileExtensions
	unread  int
	version uint64
}

type fileExtensions struct {
	rules []Registration
	// unread says the file registers rules the server can't name: extend()
	// with a computed name, Validator::resolver(), or a callback run when
	// the validator is resolved.
	unread bool
}

// NewValidationExtensions returns an empty set of extensions.
func NewValidationExtensions() *ValidationExtensions {
	return &ValidationExtensions{files: map[string]fileExtensions{}}
}

// IndexFile reads the rules a file registers; an empty source removes the
// file's.
func (e *ValidationExtensions) IndexFile(uri, source string) {
	if e == nil {
		return
	}
	found := validationExtensionsIn(uri, source)
	e.mu.Lock()
	defer e.mu.Unlock()
	old, had := e.files[uri]
	if had && old.unread == found.unread && slices.Equal(old.rules, found.rules) {
		return
	}
	if old.unread {
		e.unread--
	}
	if found.unread {
		e.unread++
	}
	if len(found.rules) == 0 && !found.unread {
		delete(e.files, uri)
	} else {
		e.files[uri] = found
	}
	if had || len(found.rules) > 0 || found.unread {
		e.version++
	}
}

func (e *ValidationExtensions) currentVersion() uint64 {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.version
}

func (e *ValidationExtensions) snapshot() (version uint64, rules []Registration, unread bool) {
	if e == nil {
		return 0, nil, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, f := range e.files {
		rules = append(rules, f.rules...)
	}
	slices.SortFunc(rules, func(a, b Registration) int {
		if c := strings.Compare(a.URI, b.URI); c != 0 {
			return c
		}
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		return a.Col - b.Col
	})
	return e.version, rules, e.unread > 0
}

func validationExtensionsIn(uri, src string) fileExtensions {
	if !namesValidatorExtension(src) {
		return fileExtensions{}
	}
	file := phpparser.Parse(src, token.Latest)
	starts := lineStarts(src)
	names := fileNames{uses: map[string]string{}}
	var found fileExtensions
	ast.Walk(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Namespace:
			names.enter(src, n)
		case *ast.Use:
			names.use(src, n)
		case *ast.StaticCall:
			class, ok := n.Class.(*ast.ConstFetch)
			if !ok {
				return true
			}
			if fqn := names.resolve(class.Name.Text(src)); fqn != validatorFacade && fqn != "Validator" {
				found.unread = found.unread || isResolvingCallback(src, names, memberName(src, n.Name), n.Args)
				return true
			}
			switch name := memberName(src, n.Name); {
			case isExtend(name):
				found.add(uri, src, starts, n.Args)
			case name == "resolver":
				found.unread = true
			}
		case *ast.MethodCall:
			switch name := memberName(src, n.Name); name {
			case "extendImplicit", "extendDependent":
				found.add(uri, src, starts, n.Args)
			case "extend":
				if isValidatorExpr(src, n.Object) {
					found.add(uri, src, starts, n.Args)
				}
			case "resolver":
				found.unread = found.unread || isValidatorExpr(src, n.Object)
			default:
				found.unread = found.unread || isResolvingCallback(src, names, name, n.Args)
			}
		}
		return true
	})
	return found
}

func isResolvingCallback(src string, names fileNames, method string, args *ast.ArgList) bool {
	switch method {
	case "resolving", "afterResolving", "callAfterResolving":
	default:
		return false
	}
	if len(args.Args) == 0 {
		return false
	}
	switch target := args.Args[0].Value.(type) {
	case *ast.Literal:
		key, ok := constantString(src, target)
		return ok && key == "validator"
	case *ast.ClassConstFetch:
		class, ok := target.Class.(*ast.ConstFetch)
		if !ok || !strings.EqualFold(memberName(src, target.Name), "class") {
			return false
		}
		switch names.resolve(class.Name.Text(src)) {
		case `Illuminate\Contracts\Validation\Factory`, `Illuminate\Validation\Factory`:
			return true
		}
	}
	return false
}

func namesValidatorExtension(src string) bool {
	if !mentionsExtend(src) && !strings.Contains(src, "resolver") && !strings.Contains(src, "esolving") {
		return false
	}
	for _, t := range lexer.Lex(src, token.Latest) {
		if t.Kind != token.Identifier {
			continue
		}
		switch name := t.Text(src); {
		case isExtend(name), name == "resolver", name == "resolving", name == "afterResolving", name == "callAfterResolving":
			return true
		}
	}
	return false
}

func mentionsExtend(src string) bool {
	for rest := src; ; {
		i := strings.Index(rest, "extend")
		if i < 0 {
			return false
		}
		rest = rest[i+len("extend"):]
		if rest == "" || rest[0] != 's' {
			return true
		}
	}
}

func isExtend(name string) bool {
	switch name {
	case "extend", "extendImplicit", "extendDependent":
		return true
	}
	return false
}

func memberName(src string, e ast.Expr) string {
	if id, ok := e.(*ast.IdentifierExpr); ok {
		return id.Text(src)
	}
	return ""
}

func isValidatorExpr(src string, e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Variable:
		return e.Text(src) == "$validator"
	case *ast.ArrayDimFetch:
		key, ok := constantString(src, e.Dim)
		return ok && key == "validator"
	case *ast.Call:
		callee, ok := e.Callee.(*ast.ConstFetch)
		if !ok || strings.TrimPrefix(callee.Name.Text(src), `\`) != "app" || len(e.Args.Args) != 1 {
			return false
		}
		key, ok := constantString(src, e.Args.Args[0].Value)
		return ok && key == "validator"
	}
	return false
}

func (f *fileExtensions) add(uri, src string, starts []int, args *ast.ArgList) {
	if len(args.Args) == 0 {
		return
	}
	name, ok := constantString(src, args.Args[0].Value)
	if !ok || name == "" {
		f.unread = true
		return
	}
	at := offsetPosition(starts, int(args.Args[0].Value.Bounds().Start))
	f.rules = append(f.rules, Registration{Name: unescapePHP(name), URI: uri, Line: at.Line, Col: at.Character})
}

type fileNames struct {
	namespace string
	uses      map[string]string
}

func (f *fileNames) enter(src string, ns *ast.Namespace) {
	f.namespace = ""
	if ns.Name != nil {
		f.namespace = ns.Name.Text(src)
	}
	clear(f.uses)
}

func (f *fileNames) use(src string, u *ast.Use) {
	prefix := ""
	if u.Prefix != nil {
		prefix = strings.Trim(u.Prefix.Text(src), `\`) + `\`
	}
	for _, item := range u.Items {
		if item.Kind != token.Use || item.Name == nil {
			continue
		}
		fqn := prefix + strings.Trim(item.Name.Text(src), `\`)
		alias := fqn[strings.LastIndexByte(fqn, '\\')+1:]
		if item.Alias != nil {
			alias = item.Alias.Text(src)
		}
		f.uses[strings.ToLower(alias)] = fqn
	}
}

func (f *fileNames) resolve(name string) string {
	if strings.HasPrefix(name, `\`) {
		return name[1:]
	}
	first, rest, qualified := strings.Cut(name, `\`)
	if fqn, ok := f.uses[strings.ToLower(first)]; ok {
		if qualified {
			return fqn + `\` + rest
		}
		return fqn
	}
	if f.namespace != "" {
		return f.namespace + `\` + name
	}
	return name
}
