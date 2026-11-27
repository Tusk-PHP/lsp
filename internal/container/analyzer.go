package container

import (
	"cmp"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

type ServiceBinding struct {
	Abstract      string
	Concrete      string
	Singleton     bool
	Source        string
	Tags          []string
	Alias         string
	DefinitionURI string
	Definition    textpos.Range
}

type ContainerAnalyzer struct {
	view      atomic.Pointer[bindingSet]
	sourcesMu sync.Mutex
	sources   map[string]source
	seq       atomic.Uint64
	index     *symbols.Index
	rootPath  string
	framework string
}

func NewContainerAnalyzer(index *symbols.Index, rootPath, framework string) *ContainerAnalyzer {
	ca := &ContainerAnalyzer{
		sources:   make(map[string]source),
		index:     index,
		rootPath:  rootPath,
		framework: framework,
	}
	ca.view.Store(newBindingSet(nil))
	return ca
}

func (ca *ContainerAnalyzer) Framework() string {
	if ca == nil {
		return ""
	}
	return ca.framework
}

func (ca *ContainerAnalyzer) RootPath() string {
	if ca == nil {
		return ""
	}
	return ca.rootPath
}

func (ca *ContainerAnalyzer) ResolveDependency(typeName string) *ServiceBinding {
	return ca.view.Load().resolve(typeName, map[string]bool{})
}

func (ca *ContainerAnalyzer) LookupBinding(name string) *ServiceBinding {
	return ca.view.Load().bindings[name]
}

// ResolveFacade checks whether classFQN is a Laravel Facade (extends
// Illuminate\Support\Facades\Facade), extracts the string returned by
// getFacadeAccessor(), and resolves it through the container bindings.
// Returns the concrete class FQN or "" if the class is not a facade or
// the accessor cannot be resolved.
func (ca *ContainerAnalyzer) ResolveFacade(classFQN string) string {
	// Check inheritance chain for the Facade base class
	chain := ca.index.GetInheritanceChain(classFQN)
	isFacade := false
	for _, parent := range chain {
		if parent == `Illuminate\Support\Facades\Facade` {
			isFacade = true
			break
		}
	}
	if !isFacade {
		return ""
	}

	// Find getFacadeAccessor method and extract its return string
	accessor := ca.extractFacadeAccessor(classFQN)
	if accessor == "" {
		return ""
	}

	// Resolve the accessor string through the container
	if binding := ca.ResolveDependency(accessor); binding != nil {
		return binding.Concrete
	}
	// The accessor might be a class FQN directly (some facades return a class name)
	if sym := ca.index.Lookup(accessor); sym != nil {
		return accessor
	}
	return ""
}

// extractFacadeAccessor reads the source of the getFacadeAccessor method on
// classFQN and returns the string literal it returns (e.g. "cache").
func (ca *ContainerAnalyzer) extractFacadeAccessor(classFQN string) string {
	members := ca.index.GetClassMembers(classFQN)
	for _, m := range members {
		if m.Name != "getFacadeAccessor" {
			continue
		}
		if m.URI == "" || m.URI == "builtin" {
			continue
		}
		path := symbols.URIToPath(m.URI)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return parseFacadeAccessorReturn(string(content), ca.index.GrammarOf(m.URI))
	}
	return ""
}

func parseFacadeAccessorReturn(source string, grammars phpparser.GrammarRange) string {
	accessor := ""
	ast.Walk(parser.ParseTreeIn(source, grammars), func(n ast.Node) bool {
		method, ok := n.(*ast.Method)
		if !ok || accessor != "" {
			return accessor == ""
		}
		if method.Name == nil || method.Body == nil || !strings.EqualFold(method.Name.Bounds().Text(source), "getFacadeAccessor") {
			return false
		}
		for _, stmt := range method.Body.Stmts {
			if ret, ok := stmt.(*ast.Return); ok && ret.Value != nil {
				accessor, _ = parser.StringLiteral(ret.Value.Bounds().Text(source))
				break
			}
		}
		return false
	})
	return accessor
}

func (ca *ContainerAnalyzer) GetBindings() map[string]*ServiceBinding {
	return maps.Clone(ca.view.Load().bindings)
}

type InjectedDependency struct {
	ParamName        string
	TypeHint         string
	ResolvedConcrete string
	IsSingleton      bool
	// AutowireKind describes the source of #[Autowire] on this parameter.
	// Values: "service", "param", "env", or "" (none / type-hint autowire).
	AutowireKind string
	// AutowireValue holds the raw value from the #[Autowire(...)] attribute:
	// the service ID, parameter name (with optional % wrapping), or env var name.
	AutowireValue string
}

// AnalyzeConstructorInjection resolves constructor parameter types for classFQN
// through the container bindings. For Symfony projects it additionally parses
// #[Autowire(...)] attributes and uses them to override the resolution with the
// explicitly declared service, parameter, or env binding.
func (ca *ContainerAnalyzer) AnalyzeConstructorInjection(classFQN string) []InjectedDependency {
	sym := ca.index.Lookup(classFQN)
	if sym == nil {
		return nil
	}

	// For Symfony projects, fetch the source file to parse Autowire attributes.
	// The Autowire list is indexed by parameter position (0-based) so it is
	// independent of whether the AST parser correctly captures param names.
	var autowireList []autowireAttr
	if ca.framework == "symfony" && sym.URI != "" && sym.URI != "builtin" {
		path := symbols.URIToPath(sym.URI)
		if data, err := os.ReadFile(path); err == nil {
			autowireList = constructorAutowire(string(data), sym.FQN, ca.index.GrammarOf(sym.URI))
		}
	}

	var deps []InjectedDependency
	for _, child := range sym.Children {
		if child.Kind != symbols.KindMethod || child.Name != "__construct" {
			continue
		}

		for idx, param := range child.Params {
			dep := InjectedDependency{ParamName: param.Name, TypeHint: param.Type}

			// Check for an explicit #[Autowire] at this parameter position.
			if idx < len(autowireList) && autowireList[idx].kind != "" {
				aw := autowireList[idx]
				dep.AutowireKind = aw.kind
				dep.AutowireValue = aw.value
				if aw.kind == "service" {
					// Resolve through the container bindings using the service ID.
					if binding := ca.ResolveDependency(aw.value); binding != nil {
						dep.ResolvedConcrete = binding.Concrete
						dep.IsSingleton = binding.Singleton
					} else {
						dep.ResolvedConcrete = aw.value
					}
				}
				// For "param" and "env" kinds there's no container binding; leave
				// ResolvedConcrete empty so callers can handle them specially.
			} else if binding := ca.ResolveDependency(param.Type); binding != nil {
				dep.ResolvedConcrete = binding.Concrete
				dep.IsSingleton = binding.Singleton
			}
			deps = append(deps, dep)
		}
	}
	return deps
}

type autowireAttr struct {
	kind  string
	value string
}

const autowireFQN = `Symfony\Component\DependencyInjection\Attribute\Autowire`

func constructorAutowire(source, classFQN string, grammars phpparser.GrammarRange) []autowireAttr {
	file := parser.ParseFileIn(source, grammars)
	for _, cls := range file.Classes {
		if !strings.EqualFold(cls.FullName, classFQN) {
			continue
		}
		for _, method := range cls.Methods {
			if !strings.EqualFold(method.Name, "__construct") {
				continue
			}
			attrs := make([]autowireAttr, len(method.Params))
			for i, param := range method.Params {
				attrs[i] = autowireOf(file, param.Attributes)
			}
			return attrs
		}
	}
	return nil
}

func autowireOf(file *parser.FileNode, attrs []parser.AttributeDef) autowireAttr {
	for _, attr := range attrs {
		if !strings.EqualFold(file.ResolveClassName(attr.Name), autowireFQN) {
			continue
		}
		for _, arg := range []struct {
			kind, name string
			position   int
		}{{"service", "service", 1}, {"param", "param", 4}, {"env", "env", 3}} {
			if value, ok := attr.Args.Get(arg.name, arg.position); ok {
				if str, ok := parser.StringLiteral(value); ok && str != "" {
					return autowireAttr{kind: arg.kind, value: str}
				}
			}
		}
		if value, ok := attr.Args.Get("value", 0); ok {
			if str, ok := parser.StringLiteral(value); ok && len(str) > 2 && strings.HasPrefix(str, "%") && strings.HasSuffix(str, "%") {
				return autowireAttr{kind: "param", value: str[1 : len(str)-1]}
			}
		}
	}
	return autowireAttr{}
}

func defaultBindings(framework string) []ServiceBinding {
	switch framework {
	case "laravel":
		return laravelDefaults
	case "symfony":
		return symfonyDefaults
	}
	return nil
}

var laravelDefaults = []ServiceBinding{
	{Abstract: "Illuminate\\Contracts\\Auth\\Factory", Concrete: "Illuminate\\Auth\\AuthManager", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Cache\\Factory", Concrete: "Illuminate\\Cache\\CacheManager", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Config\\Repository", Concrete: "Illuminate\\Config\\Repository", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Container\\Container", Concrete: "Illuminate\\Container\\Container", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Events\\Dispatcher", Concrete: "Illuminate\\Events\\Dispatcher", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Filesystem\\Factory", Concrete: "Illuminate\\Filesystem\\FilesystemManager", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Http\\Kernel", Concrete: "App\\Http\\Kernel", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Mail\\Mailer", Concrete: "Illuminate\\Mail\\Mailer", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Queue\\Factory", Concrete: "Illuminate\\Queue\\QueueManager", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Routing\\ResponseFactory", Concrete: "Illuminate\\Routing\\ResponseFactory", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Routing\\UrlGenerator", Concrete: "Illuminate\\Routing\\UrlGenerator", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Session\\Session", Concrete: "Illuminate\\Session\\Store", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\Validation\\Factory", Concrete: "Illuminate\\Validation\\Factory", Singleton: true},
	{Abstract: "Illuminate\\Contracts\\View\\Factory", Concrete: "Illuminate\\View\\Factory", Singleton: true},
	{Abstract: "auth", Concrete: "Illuminate\\Auth\\AuthManager", Singleton: true},
	{Abstract: "cache", Concrete: "Illuminate\\Cache\\CacheManager", Singleton: true},
	{Abstract: "config", Concrete: "Illuminate\\Config\\Repository", Singleton: true},
	{Abstract: "db", Concrete: "Illuminate\\Database\\DatabaseManager", Singleton: true},
	{Abstract: "events", Concrete: "Illuminate\\Events\\Dispatcher", Singleton: true},
	{Abstract: "log", Concrete: "Illuminate\\Log\\LogManager", Singleton: true},
	{Abstract: "queue", Concrete: "Illuminate\\Queue\\QueueManager", Singleton: true},
	{Abstract: "request", Concrete: "Illuminate\\Http\\Request", Singleton: true},
	{Abstract: "router", Concrete: "Illuminate\\Routing\\Router", Singleton: true},
	{Abstract: "session", Concrete: "Illuminate\\Session\\SessionManager", Singleton: true},
	{Abstract: "view", Concrete: "Illuminate\\View\\Factory", Singleton: true},
}

func (ca *ContainerAnalyzer) parseLaravelProvider(path string, content string) providerBindings {
	var bindings providerBindings
	calls := newConfigCalls(content, ca.index.Grammars())
	for _, call := range calls.named("bind", "singleton") {
		if len(call.args) < 2 || compactExpr(calls.text(call.owner)) != "$this->app" {
			continue
		}
		abstract := calls.serviceName(call.args[0].Value)
		if abstract == "" {
			continue
		}
		concrete := calls.serviceName(call.args[1].Value)
		if concrete == "" {
			concrete = abstract
		}
		bindings = append(bindings, ServiceBinding{Abstract: abstract, Concrete: concrete, Singleton: strings.EqualFold(call.method, "singleton"), Source: path})
	}
	return bindings
}

var symfonyDefaults = []ServiceBinding{
	{Abstract: "Symfony\\Component\\HttpFoundation\\RequestStack", Concrete: "Symfony\\Component\\HttpFoundation\\RequestStack", Singleton: true},
	{Abstract: "Symfony\\Component\\HttpKernel\\KernelInterface", Concrete: "App\\Kernel", Singleton: true},
	{Abstract: "Symfony\\Component\\Routing\\RouterInterface", Concrete: "Symfony\\Component\\Routing\\Router", Singleton: true},
	{Abstract: "Symfony\\Component\\EventDispatcher\\EventDispatcherInterface", Concrete: "Symfony\\Component\\EventDispatcher\\EventDispatcher", Singleton: true},
	{Abstract: "Psr\\Log\\LoggerInterface", Concrete: "Symfony\\Bridge\\Monolog\\Logger", Singleton: true},
	{Abstract: "Doctrine\\ORM\\EntityManagerInterface", Concrete: "Doctrine\\ORM\\EntityManager", Singleton: true},
	{Abstract: "Symfony\\Component\\Security\\Core\\Authorization\\AuthorizationCheckerInterface", Concrete: "Symfony\\Component\\Security\\Core\\Authorization\\AuthorizationChecker", Singleton: true},
	{Abstract: "Symfony\\Component\\Serializer\\SerializerInterface", Concrete: "Symfony\\Component\\Serializer\\Serializer", Singleton: true},
	{Abstract: "Symfony\\Component\\Validator\\Validator\\ValidatorInterface", Concrete: "Symfony\\Component\\Validator\\Validator\\RecursiveValidator", Singleton: true},
	{Abstract: "Symfony\\Component\\Mailer\\MailerInterface", Concrete: "Symfony\\Component\\Mailer\\Mailer", Singleton: true},
	{Abstract: "Symfony\\Component\\Messenger\\MessageBusInterface", Concrete: "Symfony\\Component\\Messenger\\MessageBus", Singleton: true},
	{Abstract: "Twig\\Environment", Concrete: "Twig\\Environment", Singleton: true},
}

func parseSymfonyServicesYAML(path, content string) serviceDefinitions {
	var (
		defs           serviceDefinitions
		inServices     bool
		servicesIndent = -1
		serviceIndent  = -1
		current        = -1
		currentIndent  int
		inTags         bool
	)
	for lineNo, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := leadingIndent(line)
		key, isMapping := parseYAMLMappingKey(trimmed)
		if !inServices {
			if isMapping && key == "services" {
				inServices = true
				servicesIndent = indent
				serviceIndent = -1
			}
			continue
		}
		if indent <= servicesIndent {
			inServices = false
			current = -1
			inTags = false
			serviceIndent = -1
			continue
		}
		if isMapping && indent > servicesIndent && (serviceIndent == -1 || indent == serviceIndent) {
			current = -1
			inTags = false
			if serviceIndent == -1 {
				serviceIndent = indent
			}
			currentIndent = indent
			serviceID := normalizeYAMLScalar(key)
			if !shouldTrackSymfonyServiceID(serviceID) {
				continue
			}
			defs = append(defs, serviceDefinition{id: serviceID, source: path, at: textpos.Position{Line: lineNo, Character: indent}})
			current = len(defs) - 1
			continue
		}
		if current < 0 || indent <= currentIndent {
			inTags = false
			continue
		}
		def := &defs[current]

		if inTags && strings.HasPrefix(trimmed, "-") {
			def.tags = append(def.tags, normalizeYAMLScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))))
			continue
		}
		inTags = false

		if !isMapping {
			continue
		}
		value := normalizeYAMLScalar(strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:]))
		switch key {
		case "class":
			def.class = value
		case "alias":
			def.alias = value
			def.class = cmp.Or(def.class, value)
		case "shared":
			def.sharing = sharingShared
			if value == "false" {
				def.sharing = sharingUnshared
			}
		case "tags":
			inTags = true
			if value != "" && value != "[]" {
				def.tags = append(def.tags, parseInlineYAMLList(value)...)
			}
		}
	}
	return defs
}

func (ca *ContainerAnalyzer) parseSymfonyPHPConfigFile(path, content string) serviceDefinitions {
	var defs serviceDefinitions
	calls := newConfigCalls(content, ca.index.Grammars())
	for _, call := range calls.named("set", "alias") {
		if len(call.args) == 0 {
			continue
		}
		serviceID := calls.serviceName(call.args[0].Value)
		if serviceID == "" || !shouldTrackSymfonyServiceID(serviceID) {
			continue
		}
		var target string
		if len(call.args) > 1 {
			target = calls.serviceName(call.args[1].Value)
		}
		alias := strings.EqualFold(call.method, "alias")
		if alias && target == "" {
			continue
		}
		def := serviceDefinition{id: serviceID, source: path, at: calls.position(call.args[0]), class: cmp.Or(target, serviceID)}
		if alias {
			def.alias = target
		}
		defs = append(defs, def)
	}
	return defs
}

type configCall struct {
	method string
	owner  ast.Expr
	args   []*ast.Arg
}

type configCalls struct {
	src   string
	file  *parser.FileNode
	tree  *ast.File
	lines ast.Lines
}

func newConfigCalls(content string, grammars phpparser.GrammarRange) configCalls {
	tree := parser.ParseTreeIn(content, grammars)
	return configCalls{src: content, file: parser.FileNodeOf(content, tree), tree: tree, lines: ast.NewLines(content)}
}

func (c configCalls) text(n ast.Node) string {
	return n.Bounds().Text(c.src)
}

func (c configCalls) named(methods ...string) []configCall {
	var calls []configCall
	ast.Walk(c.tree, func(n ast.Node) bool {
		call, ok := n.(*ast.MethodCall)
		if !ok || call.Args == nil {
			return true
		}
		name, ok := call.Name.(*ast.IdentifierExpr)
		if !ok {
			return true
		}
		method := c.text(name)
		if slices.ContainsFunc(methods, func(m string) bool { return strings.EqualFold(m, method) }) {
			calls = append(calls, configCall{method: method, owner: call.Object, args: call.Args.Args})
		}
		return true
	})
	return calls
}

func (c configCalls) serviceName(e ast.Expr) string {
	if e == nil {
		return ""
	}
	if value, ok := parser.StringLiteral(c.text(e)); ok {
		return value
	}
	fetch, ok := e.(*ast.ClassConstFetch)
	if !ok || !strings.EqualFold(c.text(fetch.Name), "class") {
		return ""
	}
	class, ok := fetch.Class.(*ast.ConstFetch)
	if !ok || class.Name == nil {
		return ""
	}
	return c.file.ResolveClassName(c.text(class.Name))
}

func (c configCalls) position(n ast.Node) textpos.Position {
	line, col := c.lines.Position(n.Bounds().Start)
	return textpos.Position{Line: line, Character: col}
}

func compactExpr(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
}

const asAliasFQN = `Symfony\Component\DependencyInjection\Attribute\AsAlias`

func (ca *ContainerAnalyzer) parseSymfonyClasses(path string, content string) (attributeAliases, classBindings) {
	calls := newConfigCalls(content, ca.index.Grammars())
	uri := pathToFileURI(path)
	var bindings classBindings
	for _, cls := range calls.file.Classes {
		bindings = append(bindings, ServiceBinding{Abstract: cls.FullName, Concrete: cls.FullName, Singleton: true, Source: path, DefinitionURI: uri})
		for _, iface := range calls.interfacesOf(cls) {
			bindings = append(bindings, ServiceBinding{Abstract: iface, Concrete: cls.FullName, Singleton: true, Source: path, DefinitionURI: uri})
		}
	}
	return calls.asAliases(path), bindings
}

func (c configCalls) interfacesOf(cls parser.ClassNode) []string {
	interfaces := make([]string, len(cls.Implements))
	for i, iface := range cls.Implements {
		interfaces[i] = parser.ResolveImportedName(iface, c.file.Namespace, c.file.Uses, "class")
	}
	return interfaces
}

func (c configCalls) asAliases(path string) attributeAliases {
	var aliases attributeAliases
	ast.Walk(c.tree, func(n ast.Node) bool {
		decl, ok := n.(*ast.ClassDecl)
		if !ok || decl.Name == nil {
			return true
		}
		cls := c.classAt(decl.Name.Start)
		if cls == nil {
			return true
		}
		for _, group := range decl.Attrs {
			for _, attr := range group.Attrs {
				if attr.Name == nil || !strings.EqualFold(c.file.ResolveClassName(c.text(attr.Name)), asAliasFQN) {
					continue
				}
				if id := c.asAliasID(attr, cls); id != "" {
					aliases = append(aliases, serviceDefinition{id: id, source: path, at: c.position(attr), class: cls.FullName, alias: cls.FullName})
				}
			}
		}
		return true
	})
	return aliases
}

func (c configCalls) asAliasID(attr *ast.Attribute, cls *parser.ClassNode) string {
	id := c.argument(attr.Args, "id", 0)
	if id != nil && !c.isNull(id) {
		return c.serviceName(id)
	}
	interfaces := c.interfacesOf(*cls)
	if cls.Extends != "" || len(interfaces) != 1 {
		return ""
	}
	return interfaces[0]
}

func (c configCalls) classAt(offset uint32) *parser.ClassNode {
	line, col := c.lines.Position(offset)
	for i := range c.file.Classes {
		if cls := &c.file.Classes[i]; cls.StartLine == line && cls.StartCol == col {
			return cls
		}
	}
	return nil
}

func (c configCalls) argument(list *ast.ArgList, name string, position int) ast.Expr {
	if list == nil {
		return nil
	}
	for _, arg := range list.Args {
		if arg.Name != nil && c.text(arg.Name) == name {
			return arg.Value
		}
	}
	if position < len(list.Args) && list.Args[position].Name == nil && !list.Args[position].Unpack {
		return list.Args[position].Value
	}
	return nil
}

func (c configCalls) isNull(e ast.Expr) bool {
	fetch, ok := e.(*ast.ConstFetch)
	return ok && fetch.Name != nil && strings.EqualFold(c.text(fetch.Name), "null")
}

func leadingIndent(s string) int {
	return len(s) - len(strings.TrimLeft(s, " \t"))
}

func parseYAMLMappingKey(trimmed string) (string, bool) {
	if trimmed == "" || strings.HasPrefix(trimmed, "-") {
		return "", false
	}
	idx := strings.Index(trimmed, ":")
	if idx <= 0 {
		return "", false
	}
	return strings.TrimSpace(trimmed[:idx]), true
}

func normalizeYAMLScalar(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "'\"")
	if idx := strings.Index(s, " #"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	return s
}

func shouldTrackSymfonyServiceID(serviceID string) bool {
	if serviceID == "" || strings.HasPrefix(serviceID, "_") {
		return false
	}
	if strings.HasSuffix(serviceID, "\\") {
		return false
	}
	return true
}

func parseInlineYAMLList(value string) []string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil
	}
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := normalizeYAMLScalar(part)
		if item != "" {
			items = append(items, item)
		}
	}
	sort.Strings(items)
	return items
}

func pathToFileURI(path string) string {
	return "file://" + filepath.ToSlash(path)
}
