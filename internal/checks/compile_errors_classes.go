package checks

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func (c *compileChecker) classDecl(n *ast.ClassDecl) {
	class := &classContext{kind: kindClass}
	for _, m := range n.Modifiers {
		if m.Kind == token.Abstract {
			class.abstract = true
		}
	}
	c.args(n.Args)
	c.classLike(n, n.Name, class, n.Members)
}

func (c *compileChecker) classLike(n ast.Node, name *ast.Identifier, class *classContext, members []ast.Member) {
	if name != nil {
		class.name = c.text(name)
		if c.namespace != "" {
			class.name = c.namespace + `\` + class.name
		}
		c.redeclaredMembers(class, members)
	}
	outer, outerFn := c.class, c.fn
	c.class, c.fn = class, &functionScope{inClass: true}
	ast.Children(n, c.visitChild)
	c.class, c.fn = outer, outerFn
}

func promoted(p *ast.Param) bool {
	return len(p.Modifiers) > 0
}

func (c *compileChecker) redeclaredMembers(class *classContext, members []ast.Member) {
	methods, properties, constants := map[string]bool{}, map[string]bool{}, map[string]bool{}
	property := func(v *ast.Variable) {
		name := c.text(v)[1:]
		if properties[name] {
			c.report(v, "Cannot redeclare "+class.name+"::$"+name)
		}
		properties[name] = true
	}
	constant := func(id *ast.Identifier) {
		name := c.text(id)
		if constants[name] {
			c.report(id, "Cannot redefine class constant "+class.name+"::"+name)
		}
		constants[name] = true
	}
	for _, member := range members {
		switch m := member.(type) {
		case *ast.Method:
			name := c.text(m.Name)
			if key := strings.ToLower(name); methods[key] {
				c.report(m.Name, "Cannot redeclare "+class.name+"::"+name+"()")
			} else {
				methods[key] = true
			}
			for i, p := range m.Params {
				if promoted(p) && !slices.ContainsFunc(m.Params[:i], func(q *ast.Param) bool { return c.text(q.Var) == c.text(p.Var) }) {
					property(p.Var)
				}
			}
		case *ast.Property:
			for _, item := range m.Items {
				property(item.Var)
			}
		case *ast.ClassConst:
			for _, item := range m.Items {
				constant(item.Name)
			}
		case *ast.EnumCase:
			constant(m.Name)
		}
	}
}

func hasModifier(mods []*ast.Modifier, kind token.Kind) bool {
	return slices.ContainsFunc(mods, func(m *ast.Modifier) bool { return m.Kind == kind })
}

var magicMethodRoles = map[string]string{
	"__construct": "Constructor",
	"__destruct":  "Destructor",
	"__clone":     "Clone method",
}

func (c *compileChecker) method(n *ast.Method) {
	c.params(n.Params)
	if c.class.named() {
		c.methodDecl(n)
	}
	for _, p := range n.Params {
		if promoted(p) {
			c.hooks(p.Hooks, p.Var)
		}
	}
	c.inFunction(n, &functionScope{inClass: true, returnType: n.ReturnType})
}

func (c *compileChecker) methodDisplay(n *ast.Method) string {
	return c.class.name + "::" + c.text(n.Name) + "()"
}

func (c *compileChecker) methodDecl(n *ast.Method) {
	abstract := hasModifier(n.Modifiers, token.Abstract)
	interfaceMethod := c.class.kind == kindInterface
	switch {
	case interfaceMethod:
		if n.Body != nil {
			c.report(n.Name, "Interface function "+c.methodDisplay(n)+" cannot contain body")
		}
		c.interfaceModifiers(n)
	case abstract && n.Body != nil && (c.class.abstract || c.class.kind == kindTrait):
		c.report(n.Name, "Abstract function "+c.methodDisplay(n)+" cannot contain body")
	case !abstract && n.Body == nil:
		c.report(n.Name, "Non-abstract method "+c.methodDisplay(n)+" must contain body")
	}
	lower := strings.ToLower(c.text(n.Name))
	role, magic := magicMethodRoles[lower]
	static, typed := hasModifier(n.Modifiers, token.Static), n.ReturnType != nil && lower != "__clone"
	if !magic || !static && !typed || interfaceMethod && !c.since(token.PHP80) {
		return
	}
	subject := "Method " + c.methodDisplay(n)
	if !c.since(token.PHP80) {
		subject = role + " " + c.methodDisplay(n)
	}
	if static {
		c.report(n.Name, subject+" cannot be static")
	}
	if typed {
		c.report(n.ReturnType, subject+" cannot declare a return type")
	}
}

func (c *compileChecker) interfaceModifiers(n *ast.Method) {
	nonPublic := hasModifier(n.Modifiers, token.Private) || hasModifier(n.Modifiers, token.Protected)
	final, abstract := hasModifier(n.Modifiers, token.Final), hasModifier(n.Modifiers, token.Abstract)
	switch {
	case !c.since(token.PHP81):
		if nonPublic || final || abstract {
			c.report(n.Name, "Access type for interface method "+c.methodDisplay(n)+" must be omitted")
		}
	case nonPublic:
		c.report(n.Name, "Access type for interface method "+c.methodDisplay(n)+" must be public")
	case final:
		c.report(n.Name, "Interface method "+c.methodDisplay(n)+" must not be final")
	case abstract:
		c.report(n.Name, "Interface method "+c.methodDisplay(n)+" must not be abstract")
	}
}

func (c *compileChecker) property(n *ast.Property) {
	if c.class != nil && c.class.kind == kindEnum {
		if c.since(token.PHP82) {
			c.report(n, "Enum "+c.class.name+" cannot include properties")
		} else {
			c.report(n, "Enums may not include properties")
		}
		return
	}
	if len(n.Hooks) == 0 || len(n.Items) != 1 {
		return
	}
	if hasModifier(n.Modifiers, token.Static) {
		c.report(n.Items[0].Var, "Cannot declare hooks for static property")
		return
	}
	c.hooks(n.Hooks, n.Items[0].Var)
}

func (c *compileChecker) hooks(hooks []*ast.PropertyHook, property *ast.Variable) {
	for i, h := range hooks {
		name := c.text(h.Name)
		kind := strings.ToLower(name)
		switch {
		case kind != "get" && kind != "set":
			if c.class.named() {
				c.report(h.Name, `Unknown hook "`+name+`" for property `+c.class.name+"::"+c.text(property)+`, expected "get" or "set"`)
			}
		case slices.ContainsFunc(hooks[:i], func(earlier *ast.PropertyHook) bool { return strings.EqualFold(c.text(earlier.Name), name) }):
			c.report(h.Name, `Cannot redeclare property hook "`+name+`"`)
		}
	}
}

func (c *compileChecker) enumCase(n *ast.EnumCase) {
	if c.class == nil || c.class.kind != kindEnum {
		return
	}
	display := c.text(n.Name) + " of "
	switch {
	case c.class.backed && n.Value == nil:
		c.report(n.Name, "Case "+display+"backed enum "+c.class.name+" must have a value")
	case c.class.backed || n.Value == nil:
	case c.since(token.PHP82):
		c.report(n.Name, "Case "+display+"non-backed enum "+c.class.name+" must not have a value")
	default:
		if backing, ok := c.literalType(n.Value); ok {
			c.report(n.Name, "Case "+display+"non-backed enum "+c.class.name+` must not have a value, try adding ": `+backing+`" to the enum declaration`)
		}
	}
}

func (c *compileChecker) literalType(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok {
		return "", false
	}
	switch lit.Kind {
	case token.IntegerLiteral:
		return "int", true
	case token.ConstantString:
		return "string", true
	}
	return "", false
}
