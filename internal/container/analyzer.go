package container

import (
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

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
	mu        sync.RWMutex
	bindings  map[string]*ServiceBinding
	aliases   map[string]string
	tags      map[string][]string
	index     *symbols.Index
	rootPath  string
	framework string
}

func NewContainerAnalyzer(index *symbols.Index, rootPath, framework string) *ContainerAnalyzer {
	return &ContainerAnalyzer{
		bindings:  make(map[string]*ServiceBinding),
		aliases:   make(map[string]string),
		tags:      make(map[string][]string),
		index:     index,
		rootPath:  rootPath,
		framework: framework,
	}
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

func (ca *ContainerAnalyzer) Analyze() {
	switch ca.framework {
	case "laravel":
		ca.analyzeLaravel()
	case "symfony":
		ca.analyzeSymfony()
	}
}

func (ca *ContainerAnalyzer) ResolveDependency(typeName string) *ServiceBinding {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	return ca.resolveDependencyLocked(typeName, map[string]bool{})
}

func (ca *ContainerAnalyzer) LookupBinding(name string) *ServiceBinding {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	if binding, ok := ca.bindings[name]; ok {
		return binding
	}
	return nil
}

func (ca *ContainerAnalyzer) resolveDependencyLocked(name string, seen map[string]bool) *ServiceBinding {
	if seen[name] {
		return nil
	}
	seen[name] = true
	if abstract, ok := ca.aliases[name]; ok {
		if binding := ca.resolveDependencyLocked(abstract, seen); binding != nil {
			return binding
		}
	}
	binding, ok := ca.bindings[name]
	if !ok {
		return nil
	}
	if binding.Alias != "" {
		if resolved := ca.resolveDependencyLocked(binding.Alias, seen); resolved != nil {
			return resolved
		}
	}
	return binding
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
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	result := make(map[string]*ServiceBinding, len(ca.bindings))
	for k, v := range ca.bindings {
		result[k] = v
	}
	return result
}

// AddBinding inserts or overwrites a binding in the analyzer's bindings map,
// keyed by b.Abstract (the same key used by LookupBinding and GetBindings).
// A nil b is silently ignored. This method is intended primarily for tests
// and programmatic seeding of bindings without filesystem parsing.
func (ca *ContainerAnalyzer) AddBinding(b *ServiceBinding) {
	if ca == nil || b == nil {
		return
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.bindings[b.Abstract] = b
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

func (ca *ContainerAnalyzer) analyzeLaravel() {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.registerLaravelDefaults()
	providersDir := filepath.Join(ca.rootPath, "app", "Providers")
	ca.scanDirectory(providersDir, ca.parseLaravelProvider)
}

func (ca *ContainerAnalyzer) registerLaravelDefaults() {
	defaults := []ServiceBinding{
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
	for _, b := range defaults {
		binding := b
		ca.bindings[binding.Abstract] = &binding
	}
}

func (ca *ContainerAnalyzer) parseLaravelProvider(path string, content string) {
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
		ca.bindings[abstract] = &ServiceBinding{Abstract: abstract, Concrete: concrete, Singleton: strings.EqualFold(call.method, "singleton"), Source: path}
	}
}

func (ca *ContainerAnalyzer) analyzeSymfony() {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.registerSymfonyDefaults()
	ca.parseSymfonyServicesYAML()
	ca.parseSymfonyServicesPHP()
	ca.scanDirectory(filepath.Join(ca.rootPath, "src"), ca.parseSymfonyAttributes)
	ca.resolveSymfonyAliases()
}

func (ca *ContainerAnalyzer) registerSymfonyDefaults() {
	defaults := []ServiceBinding{
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
	for _, b := range defaults {
		binding := b
		ca.bindings[binding.Abstract] = &binding
	}
}

func (ca *ContainerAnalyzer) parseSymfonyServicesYAML() {
	for _, name := range []string{"services.yaml", "services.yml"} {
		yamlPath := filepath.Join(ca.rootPath, "config", name)
		content, err := os.ReadFile(yamlPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		var (
			inServices     bool
			servicesIndent = -1
			serviceIndent  = -1
			currentService string
			currentIndent  int
			inTags         bool
		)
		for lineNo, line := range lines {
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
				currentService = ""
				inTags = false
				serviceIndent = -1
				continue
			}
			if isMapping && indent > servicesIndent && (serviceIndent == -1 || indent == serviceIndent) {
				currentService = ""
				inTags = false
				if serviceIndent == -1 {
					serviceIndent = indent
				}
				currentIndent = indent
				serviceID := normalizeYAMLScalar(key)
				if !shouldTrackSymfonyServiceID(serviceID) {
					continue
				}
				currentService = serviceID
				binding := ca.ensureSymfonyBinding(serviceID, yamlPath, textpos.Position{
					Line:      lineNo,
					Character: len(line) - len(strings.TrimLeft(line, " \t")),
				})
				if binding.Concrete == "" {
					binding.Concrete = serviceID
				}
				continue
			}
			if currentService == "" || indent <= currentIndent {
				inTags = false
				continue
			}

			if inTags && strings.HasPrefix(trimmed, "-") {
				if binding, ok := ca.bindings[currentService]; ok {
					binding.Tags = append(binding.Tags, normalizeYAMLScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))))
				}
				continue
			}
			inTags = false

			if !isMapping {
				continue
			}
			value := normalizeYAMLScalar(strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:]))
			switch key {
			case "class":
				if b, ok := ca.bindings[currentService]; ok {
					b.Concrete = value
				}
			case "alias":
				if b, ok := ca.bindings[currentService]; ok {
					b.Alias = value
					if b.Concrete == "" {
						b.Concrete = value
					}
					ca.aliases[currentService] = value
				}
			case "shared":
				if b, ok := ca.bindings[currentService]; ok {
					b.Singleton = value != "false"
				}
			case "tags":
				inTags = true
				if value != "" && value != "[]" {
					if b, ok := ca.bindings[currentService]; ok {
						b.Tags = append(b.Tags, parseInlineYAMLList(value)...)
					}
				}
			}
		}
	}
}

func (ca *ContainerAnalyzer) parseSymfonyServicesPHP() {
	configDir := filepath.Join(ca.rootPath, "config")
	filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		ca.parseSymfonyPHPConfigFile(path, string(content))
		return nil
	})
}

func (ca *ContainerAnalyzer) parseSymfonyPHPConfigFile(path, content string) {
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
		binding := ca.ensureSymfonyBinding(serviceID, path, calls.position(call.args[0]))
		if alias {
			binding.Alias = target
			ca.aliases[serviceID] = target
		}
		binding.Concrete = cmp.Or(target, serviceID)
	}
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

func (ca *ContainerAnalyzer) resolveSymfonyAliases() {
	for serviceID, binding := range ca.bindings {
		if binding.Alias == "" || binding.Concrete != binding.Alias {
			continue
		}
		if resolved := ca.resolveDependencyLocked(serviceID, map[string]bool{}); resolved != nil {
			binding.Concrete = resolved.Concrete
			binding.Singleton = resolved.Singleton
		}
	}
}

func (ca *ContainerAnalyzer) ensureSymfonyBinding(serviceID, sourcePath string, pos textpos.Position) *ServiceBinding {
	if binding, ok := ca.bindings[serviceID]; ok {
		if binding.DefinitionURI == "" {
			binding.DefinitionURI = pathToFileURI(sourcePath)
			binding.Definition = textpos.Range{
				Start: pos,
				End:   textpos.Position{Line: pos.Line, Character: pos.Character + len(serviceID)},
			}
		}
		if binding.Source == "" {
			binding.Source = sourcePath
		}
		return binding
	}
	binding := &ServiceBinding{
		Abstract:      serviceID,
		Concrete:      serviceID,
		Singleton:     true,
		Source:        sourcePath,
		DefinitionURI: pathToFileURI(sourcePath),
		Definition: textpos.Range{
			Start: pos,
			End:   textpos.Position{Line: pos.Line, Character: pos.Character + len(serviceID)},
		},
	}
	ca.bindings[serviceID] = binding
	return binding
}

func (ca *ContainerAnalyzer) parseSymfonyAttributes(path string, content string) {
	if !strings.HasSuffix(path, ".php") {
		return
	}
	file := parser.ParseFileIn(content, ca.index.Grammars())
	if file == nil {
		return
	}
	ns := file.Namespace
	for _, cls := range file.Classes {
		fqn := ns + "\\" + cls.Name
		if strings.Contains(path, "src"+string(filepath.Separator)) {
			if _, exists := ca.bindings[fqn]; !exists {
				ca.bindings[fqn] = &ServiceBinding{Abstract: fqn, Concrete: fqn, Singleton: true, Source: path, DefinitionURI: pathToFileURI(path)}
			}
		}
		for _, iface := range cls.Implements {
			ifaceFQN := parser.ResolveImportedName(iface, ns, file.Uses, "class")
			if _, exists := ca.bindings[ifaceFQN]; !exists {
				ca.bindings[ifaceFQN] = &ServiceBinding{Abstract: ifaceFQN, Concrete: fqn, Singleton: true, Source: path, DefinitionURI: pathToFileURI(path)}
			}
		}
	}
}

func (ca *ContainerAnalyzer) scanDirectory(dir string, handler func(string, string)) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".php") && !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		handler(path, string(content))
		return nil
	})
}

type ComposerAutoload struct {
	PSR4 map[string]string
}

func ParseComposerAutoload(rootPath string) *ComposerAutoload {
	composerPath := filepath.Join(rootPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil
	}
	var composer struct {
		Autoload struct {
			PSR4 map[string]interface{} `json:"psr-4"`
		} `json:"autoload"`
	}
	if json.Unmarshal(data, &composer) != nil {
		return nil
	}
	result := &ComposerAutoload{PSR4: make(map[string]string)}
	for ns, paths := range composer.Autoload.PSR4 {
		switch v := paths.(type) {
		case string:
			result.PSR4[ns] = v
		case []interface{}:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					result.PSR4[ns] = s
				}
			}
		}
	}
	return result
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
