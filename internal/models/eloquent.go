package models

import (
	"os"
	"strings"
	"unicode"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// Eloquent relation types grouped by cardinality.
var singularRelations = map[string]bool{
	"HasOne": true, "BelongsTo": true, "MorphOne": true, "MorphTo": true,
	"HasOneThrough": true,
}

var pluralRelations = map[string]bool{
	"HasMany": true, "BelongsToMany": true, "MorphMany": true,
	"MorphToMany": true, "MorphedByMany": true, "HasManyThrough": true,
}

// allRelationTypes is the union of singular and plural for quick lookup.
var allRelationTypes map[string]bool

func init() {
	allRelationTypes = make(map[string]bool, len(singularRelations)+len(pluralRelations))
	for k := range singularRelations {
		allRelationTypes[k] = true
	}
	for k := range pluralRelations {
		allRelationTypes[k] = true
	}
}

// eloquentStaticForwards are static methods injected on Eloquent models that
// no class declares: the SoftDeletes scope adds them to the builder at
// runtime. Everything else a model forwards to its query builder (where(),
// find(), first(), orderBy(), …) is looked up on the builder itself, through
// the Model → Builder<static> call forward the Laravel framework registers
// (framework/laravel.CallForwards). Return types use {MODEL} for the model.
var eloquentStaticForwards = []struct {
	name       string
	returnType string
	params     []symbols.ParamInfo
}{
	{"withTrashed", frameworklaravel.EloquentBuilder + "<{MODEL}>", nil},
	{"onlyTrashed", frameworklaravel.EloquentBuilder + "<{MODEL}>", nil},
}

// CastDependencies maps the URI of a file declaring models to the classes
// its models' attribute cast types were read from.
type CastDependencies map[string][]string

// AnalyzeEloquentModels scans all classes extending Eloquent Model and injects
// virtual properties for relations, accessors and attribute casts.
func AnalyzeEloquentModels(index *symbols.Index, rootPath string) CastDependencies {
	dependencies := CastDependencies{}
	for _, model := range index.GetDescendants(frameworklaravel.EloquentModel) {
		injectEloquentStaticMethods(index, model)
		if classes := analyzeModel(index, model); len(classes) > 0 {
			dependencies[model.URI] = append(dependencies[model.URI], classes...)
		}
	}
	return dependencies
}

// injectEloquentStaticMethods adds common Builder methods as virtual static
// methods on the model class, mimicking __callStatic forwarding.
func injectEloquentStaticMethods(index *symbols.Index, model *symbols.Symbol) {
	for _, m := range eloquentStaticForwards {
		// Skip if already exists (from IDE helper or real declaration)
		if index.Lookup(model.FQN+"::"+m.name) != nil {
			continue
		}

		retType := m.returnType
		if retType == "static" {
			retType = model.FQN
		}
		// Replace {MODEL} placeholder with the actual model FQN
		retType = strings.ReplaceAll(retType, "{MODEL}", model.FQN)

		index.AddVirtualMember(model.FQN, &symbols.Symbol{
			Name:       m.name,
			FQN:        model.FQN + "::" + m.name,
			Kind:       symbols.KindMethod,
			URI:        model.URI,
			Visibility: "public",
			IsStatic:   true,
			ReturnType: retType,
			Params:     m.params,
			IsVirtual:  true,
			DocComment: "Eloquent query builder method",
		})
	}
}

func analyzeModel(index *symbols.Index, model *symbols.Symbol) []string {
	content, err := os.ReadFile(symbols.URIToPath(model.URI))
	if err != nil {
		return nil
	}
	return analyzeModelSource(index, model, newModelSource(string(content), index.GrammarOf(model.URI)))
}

type modelSource struct {
	src     string
	file    *parser.FileNode
	classes map[string]*ast.ClassDecl
}

func newModelSource(source string, grammars phpparser.GrammarRange) modelSource {
	tree := parser.ParseTreeIn(source, grammars)
	return modelSource{src: source, file: parser.FileNodeOf(source, tree), classes: parser.DeclaredClasses(source, tree)}
}

func (m modelSource) class(fqn string) (*parser.ClassNode, map[string]*ast.Method) {
	for i := range m.file.Classes {
		if m.file.Classes[i].FullName != fqn {
			continue
		}
		methods := map[string]*ast.Method{}
		for _, member := range m.classes[fqn].Members {
			if method, ok := member.(*ast.Method); ok && method.Name != nil {
				methods[method.Name.Bounds().Text(m.src)] = method
			}
		}
		return &m.file.Classes[i], methods
	}
	return nil, nil
}

func (m modelSource) resolve(name string) string {
	return resolveWithUses(name, m.file.Namespace, m.file.Uses)
}

type modelRelationMethod struct {
	kind    string
	target  string
	through string
	class   string
}

func (m modelSource) relation(method parser.MethodNode, decl *ast.Method) (modelRelationMethod, bool) {
	kind := shortClassName(method.ReturnType.Name)
	if !allRelationTypes[kind] {
		kind = ""
	}
	call := m.relationCall(decl)
	if kind == "" && call != nil && (method.ReturnType.Name == "" || method.ReturnType.Name == "mixed") {
		kind = ucFirst(m.text(call.Name))
	}
	if kind == "" {
		return modelRelationMethod{}, false
	}
	relation := modelRelationMethod{kind: kind}
	if call != nil {
		relation.target = m.classArgument(call, 0)
		relation.through = m.classArgument(call, 1)
		relation.class = relationCallClasses[m.text(call.Name)]
	}
	return relation, true
}

func (m modelSource) classArgument(call *ast.MethodCall, position int) string {
	if len(call.Args.Args) <= position {
		return ""
	}
	fetch, ok := call.Args.Args[position].Value.(*ast.ClassConstFetch)
	if !ok {
		return ""
	}
	return m.classConst(fetch)
}

func (m modelSource) classConst(fetch *ast.ClassConstFetch) string {
	if !strings.EqualFold(m.text(fetch.Name), "class") {
		return ""
	}
	class, ok := fetch.Class.(*ast.ConstFetch)
	if !ok || class.Name == nil {
		return ""
	}
	return m.resolve(m.text(class.Name))
}

func (r modelRelationMethod) returnType(declaring string) string {
	if r.class == "" {
		return ""
	}
	fqn := frameworklaravel.EloquentRelations + r.class
	switch {
	case r.target == "":
		return fqn
	case throughRelations[r.class]:
		if r.through == "" {
			return fqn
		}
		return fqn + "<" + r.target + ", " + r.through + ", " + declaring + ">"
	default:
		return fqn + "<" + r.target + ", " + declaring + ">"
	}
}

func (m modelSource) text(n ast.Node) string {
	return n.Bounds().Text(m.src)
}

func (m modelSource) relationCall(decl *ast.Method) *ast.MethodCall {
	if decl == nil || decl.Body == nil {
		return nil
	}
	var found *ast.MethodCall
	ast.Walk(decl.Body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.MethodCall)
		if !ok {
			return true
		}
		name, named := call.Name.(*ast.IdentifierExpr)
		if owner, isVar := call.Object.(*ast.Variable); named && isVar && m.text(owner) == "$this" && relationCallClasses[m.text(name)] != "" {
			found = call
			return false
		}
		return true
	})
	return found
}

var relationCallClasses = map[string]string{
	"hasOne": "HasOne", "hasMany": "HasMany", "belongsTo": "BelongsTo", "belongsToMany": "BelongsToMany",
	"morphOne": "MorphOne", "morphMany": "MorphMany", "morphTo": "MorphTo", "morphToMany": "MorphToMany",
	"morphedByMany": "MorphToMany", "hasOneThrough": "HasOneThrough", "hasManyThrough": "HasManyThrough",
}

var throughRelations = map[string]bool{"HasOneThrough": true, "HasManyThrough": true}

// analyzeModelSource injects the relations, accessors and attribute casts
// model declares in source, the text of the file that declares it, and
// returns the classes its cast types were read from.
func analyzeModelSource(index *symbols.Index, model *symbols.Symbol, source modelSource) []string {
	classNode, decls := source.class(model.FQN)
	if classNode == nil {
		return nil
	}
	for _, method := range classNode.Methods {
		if relation, ok := source.relation(method, decls[method.Name]); ok {
			injectRelation(index, model, method.Name, methodNameRange(method), relation)
			if returnType := relation.returnType(model.FQN); returnType != "" && method.ReturnType.Name == "" && !hasDocReturn(method) {
				index.RefineMethodReturnType(model.FQN+"::"+method.Name, returnType)
			}
			continue
		}
		if name, ok := legacyAccessorName(method.Name); ok {
			propName := "$" + snakeCase(name)
			retType := method.ReturnType.Name
			if retType == "" && method.DocComment != "" {
				if doc := parser.ParseDocBlock(method.DocComment); doc != nil && doc.Return.Type != "" {
					retType = doc.Return.Type
				}
			}
			index.AddVirtualMember(model.FQN, &symbols.Symbol{
				Name:       propName,
				FQN:        model.FQN + "::" + propName,
				Kind:       symbols.KindProperty,
				URI:        model.URI,
				Visibility: "public",
				Type:       source.resolve(retType),
				IsVirtual:  true,
				DocComment: "Accessor from " + method.Name + "()",
				Range:      methodNameRange(method),
			})
			continue
		}
		if source.resolve(method.ReturnType.Name) == frameworklaravel.EloquentAttribute {
			propName := "$" + method.Name
			index.AddVirtualMember(model.FQN, &symbols.Symbol{
				Name:       propName,
				FQN:        model.FQN + "::" + propName,
				Kind:       symbols.KindProperty,
				URI:        model.URI,
				Visibility: "public",
				Type:       source.attributeType(method, decls[method.Name]),
				IsVirtual:  true,
				DocComment: "Accessor from " + method.Name + "()",
				Range:      methodNameRange(method),
			})
		}
	}
	return injectCasts(index, model, source, source.classes[model.FQN], decls)
}

func hasDocReturn(method parser.MethodNode) bool {
	if method.DocComment == "" {
		return false
	}
	doc := parser.ParseDocBlock(method.DocComment)
	return doc != nil && doc.Return.Type != ""
}

func legacyAccessorName(method string) (string, bool) {
	name, ok := strings.CutPrefix(method, "get")
	if !ok {
		return "", false
	}
	name, ok = strings.CutSuffix(name, "Attribute")
	if !ok || name == "" || name[0] < 'A' || name[0] > 'Z' {
		return "", false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return "", false
		}
	}
	return name, true
}

func injectRelation(index *symbols.Index, model *symbols.Symbol, methodName string, methodRange textpos.Range, relation modelRelationMethod) {
	propType := "mixed"
	if relation.target != "" {
		if singularRelations[relation.kind] {
			propType = "?" + relation.target
		} else {
			propType = frameworklaravel.EloquentCollection + "<int, " + relation.target + ">"
		}
	}
	propName := "$" + methodName
	index.AddVirtualMember(model.FQN, &symbols.Symbol{
		Name:       propName,
		FQN:        model.FQN + "::" + propName,
		Kind:       symbols.KindProperty,
		URI:        model.URI,
		Visibility: "public",
		Type:       propType,
		IsVirtual:  true,
		DocComment: relation.kind + " relation",
		Range:      methodRange,
	})
}

// methodNameRange is where the index puts the method's own symbol. A
// property made from a method (a relation, an accessor) points there, so go
// to definition on $user->posts opens posts() instead of the top of the file.
func methodNameRange(m parser.MethodNode) textpos.Range {
	return textpos.Range{
		Start: textpos.Position{Line: m.StartLine, Character: m.StartCol},
		End:   textpos.Position{Line: m.StartLine, Character: m.StartCol + len(m.Name)},
	}
}

// resolveWithUses resolves a type name using the file's use statements.
func resolveWithUses(name, ns string, uses []parser.UseNode) string {
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "?") {
		return "?" + resolveWithUses(name[1:], ns, uses)
	}
	if symbols.IsPHPBuiltinType(strings.ToLower(name)) {
		return name
	}
	return parser.ResolveImportedName(name, ns, uses, "class")
}

// shortClassName extracts the short class name from a potentially qualified name.
func shortClassName(name string) string {
	if i := strings.LastIndex(name, "\\"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// snakeCase converts PascalCase to snake_case.
func snakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ucFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
