package models

import (
	"os"
	"regexp"
	"strings"
	"unicode"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
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

// Regex to extract $this->hasMany(Post::class) style calls.
var relationCallRe = regexp.MustCompile(
	`\$this\s*->\s*(hasOne|hasMany|belongsTo|belongsToMany|morphOne|morphMany|morphTo|morphToMany|morphedByMany|hasOneThrough|hasManyThrough)\s*\(\s*([A-Za-z_\\]+)::class`,
)

// Regex for legacy accessor: getNameAttribute()
var legacyAccessorRe = regexp.MustCompile(`^get([A-Z][A-Za-z0-9]*)Attribute$`)

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

// AnalyzeEloquentModels scans all classes extending Eloquent Model and injects
// virtual properties for relations and accessors/mutators.
func AnalyzeEloquentModels(index *symbols.Index, rootPath string) {
	models := index.GetDescendants(frameworklaravel.EloquentModel)
	for _, model := range models {
		injectEloquentStaticMethods(index, model)
		analyzeModel(index, model)
	}
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

func analyzeModel(index *symbols.Index, model *symbols.Symbol) {
	// Read the source file to extract method bodies
	path := symbols.URIToPath(model.URI)
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	source := string(content)
	file := parser.ParseFileIn(source, index.GrammarOf(model.URI))
	if file == nil {
		return
	}
	analyzeModelSource(index, model, source, file)
}

// analyzeModelSource injects the relations and accessors model declares in
// source, the text of the file that declares it.
func analyzeModelSource(index *symbols.Index, model *symbols.Symbol, source string, file *parser.FileNode) {
	lines := strings.Split(source, "\n")

	// Find the class in the parsed file
	var classNode *parser.ClassNode
	for i := range file.Classes {
		fqn := file.Namespace + "\\" + file.Classes[i].Name
		if file.Namespace == "" {
			fqn = file.Classes[i].Name
		}
		if fqn == model.FQN {
			classNode = &file.Classes[i]
			break
		}
	}
	if classNode == nil {
		return
	}

	resolve := func(name string) string {
		return resolveWithUses(name, file.Namespace, file.Uses)
	}

	for _, method := range classNode.Methods {
		// Check for relation return type
		returnShort := shortClassName(method.ReturnType.Name)
		if allRelationTypes[returnShort] {
			body := extractMethodBody(lines, method.StartLine, method.EndLine)
			injectRelation(index, model, method.Name, methodNameRange(method), returnShort, body, resolve)
			continue
		}

		// Check for relation calls in body (no explicit return type)
		if method.ReturnType.Name == "" || method.ReturnType.Name == "mixed" {
			body := extractMethodBody(lines, method.StartLine, method.EndLine)
			if match := relationCallRe.FindStringSubmatch(body); match != nil {
				relType := match[1]
				relShort := ucFirst(relType)
				injectRelation(index, model, method.Name, methodNameRange(method), relShort, body, resolve)
				continue
			}
		}

		// Check for legacy accessor: getNameAttribute() → virtual $name
		if m := legacyAccessorRe.FindStringSubmatch(method.Name); m != nil {
			propName := "$" + snakeCase(m[1])
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
				Type:       resolve(retType),
				IsVirtual:  true,
				DocComment: "Accessor from " + method.Name + "()",
				Range:      methodNameRange(method),
			})
			continue
		}

		// Check for modern accessor: protected function name(): Attribute
		resolvedReturn := resolve(method.ReturnType.Name)
		if resolvedReturn == "Illuminate\\Database\\Eloquent\\Casts\\Attribute" {
			propName := "$" + method.Name
			// Try to infer type from docblock @return
			retType := ""
			if method.DocComment != "" {
				if doc := parser.ParseDocBlock(method.DocComment); doc != nil && doc.Return.Type != "" {
					retType = doc.Return.Type
				}
			}
			if retType == "" {
				retType = "mixed"
			}
			index.AddVirtualMember(model.FQN, &symbols.Symbol{
				Name:       propName,
				FQN:        model.FQN + "::" + propName,
				Kind:       symbols.KindProperty,
				URI:        model.URI,
				Visibility: "public",
				Type:       resolve(retType),
				IsVirtual:  true,
				DocComment: "Accessor from " + method.Name + "()",
				Range:      methodNameRange(method),
			})
		}
	}
}

// injectRelation creates virtual property and ensures the method also has the right return type.
func injectRelation(index *symbols.Index, model *symbols.Symbol, methodName string, methodRange protocol.Range, relType, body string, resolve func(string) string) {
	// Extract related model from body: $this->hasMany(Post::class)
	relatedModel := ""
	if match := relationCallRe.FindStringSubmatch(body); match != nil {
		relatedModel = resolve(match[2])
	}

	// Determine property type based on relation cardinality
	propType := "mixed"
	if relatedModel != "" {
		if singularRelations[relType] {
			propType = "?" + relatedModel
		} else {
			propType = "Illuminate\\Database\\Eloquent\\Collection"
		}
	}

	// Create virtual property (e.g. $user->posts as Collection)
	propName := "$" + methodName
	index.AddVirtualMember(model.FQN, &symbols.Symbol{
		Name:       propName,
		FQN:        model.FQN + "::" + propName,
		Kind:       symbols.KindProperty,
		URI:        model.URI,
		Visibility: "public",
		Type:       propType,
		IsVirtual:  true,
		DocComment: relType + " relation",
		Range:      methodRange,
	})
}

// methodNameRange is where the index puts the method's own symbol. A
// property made from a method (a relation, an accessor) points there, so go
// to definition on $user->posts opens posts() instead of the top of the file.
func methodNameRange(m parser.MethodNode) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{Line: m.StartLine, Character: m.StartCol},
		End:   protocol.Position{Line: m.StartLine, Character: m.StartCol + len(m.Name)},
	}
}

// extractMethodBody returns the source text between startLine and endLine (0-indexed lines array).
func extractMethodBody(lines []string, startLine, endLine int) string {
	// Parser lines are 0-indexed
	if startLine < 0 || endLine < startLine {
		return ""
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}
	return strings.Join(lines[startLine:endLine+1], "\n")
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
