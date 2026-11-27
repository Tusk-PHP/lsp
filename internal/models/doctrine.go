package models

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Doctrine SQL type → PHP type mapping.
var doctrineTypeMap = map[string]string{
	"string":             "string",
	"text":               "string",
	"integer":            "int",
	"smallint":           "int",
	"bigint":             "int",
	"boolean":            "bool",
	"decimal":            "string",
	"float":              "float",
	"datetime":           "\\DateTimeInterface",
	"datetime_immutable": "\\DateTimeImmutable",
	"datetimetz":         "\\DateTimeInterface",
	"date":               "\\DateTimeInterface",
	"date_immutable":     "\\DateTimeImmutable",
	"time":               "\\DateTimeInterface",
	"time_immutable":     "\\DateTimeImmutable",
	"array":              "array",
	"simple_array":       "array",
	"json":               "array",
	"json_array":         "array",
	"object":             "object",
	"binary":             "string",
	"blob":               "string",
	"guid":               "string",
}

var (
	ormEntityAnnotRe = regexp.MustCompile(`@ORM\\Entity\b(?:\(([^)]*)\))?`)
	repoClassRe      = regexp.MustCompile(`repositoryClass\s*[:=]\s*([A-Za-z_\\]+)::class`)
)

const (
	doctrineMapping    = `Doctrine\ORM\Mapping\`
	doctrineCollection = `Doctrine\Common\Collections\Collection`
)

var doctrineRelationKinds = []string{"OneToOne", "OneToMany", "ManyToOne", "ManyToMany"}

type doctrineEntity struct {
	class   *parser.ClassNode
	fqn     string
	resolve func(string) string
	entity  *parser.AttributeDef
}

func doctrineEntities(file *parser.FileNode) []doctrineEntity {
	resolve := func(name string) string {
		return resolveWithUses(name, file.Namespace, file.Uses)
	}
	var entities []doctrineEntity
	for i := range file.Classes {
		cls := &file.Classes[i]
		entity := doctrineEntity{class: cls, fqn: cls.FullName, resolve: resolve}
		entity.entity = entity.attribute(cls.Attributes, "Entity")
		if entity.entity != nil || ormEntityAnnotRe.MatchString(cls.DocComment) {
			entities = append(entities, entity)
		}
	}
	return entities
}

func (e doctrineEntity) attribute(attrs []parser.AttributeDef, kind string) *parser.AttributeDef {
	for i := range attrs {
		if strings.EqualFold(e.resolve(attrs[i].Name), doctrineMapping+kind) {
			return &attrs[i]
		}
	}
	return nil
}

func (e doctrineEntity) classArgument(attr *parser.AttributeDef, name string, position int) string {
	if attr == nil {
		return ""
	}
	value, _ := attr.Args.Get(name, position)
	class, ok := strings.CutSuffix(value, "::class")
	if !ok || class == "" {
		return ""
	}
	return e.resolve(class)
}

type doctrineRelationProperty struct {
	name   string
	kind   string
	target string
}

func (e doctrineEntity) relation(prop parser.PropertyNode) (doctrineRelationProperty, bool) {
	for _, kind := range doctrineRelationKinds {
		if attr := e.attribute(prop.Attributes, kind); attr != nil {
			return doctrineRelationProperty{name: prop.Name, kind: kind, target: e.classArgument(attr, "targetEntity", -1)}, true
		}
	}
	return doctrineRelationProperty{}, false
}

func (e doctrineEntity) repositoryClass() string {
	if repo := e.classArgument(e.entity, "repositoryClass", 0); repo != "" {
		return repo
	}
	if m := ormEntityAnnotRe.FindStringSubmatch(e.class.DocComment); m != nil && m[1] != "" {
		if rm := repoClassRe.FindStringSubmatch(m[1]); rm != nil {
			return e.resolve(rm[1])
		}
	}
	return ""
}

func (e doctrineEntity) tableName() string {
	if table := e.attribute(e.class.Attributes, "Table"); table != nil {
		if name, ok := table.Args.Get("name", 0); ok {
			if unquoted, ok := parser.StringLiteral(name); ok && unquoted != "" {
				return unquoted
			}
		}
	}
	return toSnakeCase(e.class.Name)
}

// Singular Doctrine relations (result is nullable entity).
var doctrineSingularRelations = map[string]bool{
	"OneToOne": true, "ManyToOne": true,
}

// Plural Doctrine relations (result is Collection).
var doctrinePluralRelations = map[string]bool{
	"OneToMany": true, "ManyToMany": true,
}

// AnalyzeDoctrineEntities scans all classes that have ORM\Entity attributes
// and injects virtual members for columns, relations, and repository magic methods.
func AnalyzeDoctrineEntities(index *symbols.Index, rootPath string) {
	uris := index.GetAllFileURIs()
	for _, uri := range uris {
		path := symbols.URIToPath(uri)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		analyzeDoctrineSource(index, rootPath, string(content))
	}
}

// analyzeDoctrineSource injects the members of the Doctrine entities declared
// in source, one file's text.
func analyzeDoctrineSource(index *symbols.Index, rootPath, source string) {
	if !strings.Contains(source, doctrineMapping[:len(doctrineMapping)-1]) {
		return
	}
	for _, entity := range doctrineEntities(parser.ParseFileIn(source, index.Grammars())) {
		sym := index.Lookup(entity.fqn)
		if sym == nil {
			continue
		}
		if !analyzeDoctrineAttributes(index, sym, entity) {
			analyzeDoctrineXML(index, sym, rootPath, entity.fqn, entity.class, entity.resolve)
		}
		if repoFQN := entity.repositoryClass(); repoFQN != "" {
			injectRepositoryMagicMethods(index, repoFQN, entity.fqn, sym)
		}
	}
}

func analyzeDoctrineAttributes(index *symbols.Index, entitySym *symbols.Symbol, entity doctrineEntity) bool {
	found := false
	for _, prop := range entity.class.Properties {
		if column := entity.attribute(prop.Attributes, "Column"); column != nil {
			found = true
			readonly := entity.attribute(prop.Attributes, "Id") != nil || entity.attribute(prop.Attributes, "GeneratedValue") != nil
			index.RefineProperty(entitySym.FQN+"::"+prop.Name, resolveColumnType(column, prop.Type.Name), readonly)
			continue
		}
		if relation, ok := entity.relation(prop); ok {
			found = true
			typeDoctrineRelation(index, entitySym, relation)
		}
	}
	return found
}

// resolveColumnType maps Doctrine column type to PHP type.
func resolveColumnType(column *parser.AttributeDef, propType string) string {
	if propType != "" && propType != "mixed" {
		return propType
	}
	value, _ := column.Args.Get("type", 1)
	name, ok := parser.StringLiteral(value)
	if !ok {
		constant, isConstant := strings.CutPrefix(value, "Types::")
		name, ok = strings.ToLower(constant), isConstant
	}
	if phpType, known := doctrineTypeMap[name]; ok && known {
		return phpType
	}
	return "mixed"
}

func typeDoctrineRelation(index *symbols.Index, entitySym *symbols.Symbol, relation doctrineRelationProperty) {
	propType := "mixed"
	switch {
	case doctrineSingularRelations[relation.kind] && relation.target != "":
		propType = "?" + relation.target
	case doctrinePluralRelations[relation.kind]:
		propType = doctrineToManyType(relation.target)
	}
	index.RefineProperty(entitySym.FQN+"::"+relation.name, propType, false)
}

func doctrineToManyType(target string) string {
	if target == "" {
		return doctrineCollection
	}
	return doctrineCollection + "<int, " + target + ">"
}

// injectRepositoryMagicMethods injects findBy*, findOneBy*, countBy* methods
// into a repository class based on the entity's mapped columns.
func injectRepositoryMagicMethods(index *symbols.Index, repoFQN, entityFQN string, entitySym *symbols.Symbol) {
	repoSym := index.Lookup(repoFQN)
	if repoSym == nil {
		return
	}

	// Inject base EntityRepository methods
	baseMethods := []struct {
		name       string
		returnType string
		params     []symbols.ParamInfo
	}{
		{"find", "?" + entityFQN, []symbols.ParamInfo{{Name: "$id", Type: "mixed"}}},
		{"findAll", "array", nil},
		{"findBy", "array", []symbols.ParamInfo{{Name: "$criteria", Type: "array"}, {Name: "$orderBy", Type: "?array"}, {Name: "$limit", Type: "?int"}, {Name: "$offset", Type: "?int"}}},
		{"findOneBy", "?" + entityFQN, []symbols.ParamInfo{{Name: "$criteria", Type: "array"}}},
		{"count", "int", []symbols.ParamInfo{{Name: "$criteria", Type: "array"}}},
	}

	for _, bm := range baseMethods {
		index.AddVirtualMember(repoFQN, &symbols.Symbol{
			Name:       bm.name,
			FQN:        repoFQN + "::" + bm.name,
			Kind:       symbols.KindMethod,
			URI:        repoSym.URI,
			Visibility: "public",
			ReturnType: bm.returnType,
			Params:     bm.params,
			IsVirtual:  true,
			DocComment: "EntityRepository method",
		})
	}

	// Collect column names from entity's children
	members := index.GetClassMembers(entityFQN)
	for _, m := range members {
		if m.Kind != symbols.KindProperty {
			continue
		}
		colName := strings.TrimPrefix(m.Name, "$")
		ucName := ucFirst(colName)

		// findByColumnName
		index.AddVirtualMember(repoFQN, &symbols.Symbol{
			Name:       "findBy" + ucName,
			FQN:        repoFQN + "::findBy" + ucName,
			Kind:       symbols.KindMethod,
			URI:        repoSym.URI,
			Visibility: "public",
			ReturnType: "array",
			Params:     []symbols.ParamInfo{{Name: "$value", Type: m.Type}},
			IsVirtual:  true,
			DocComment: "Magic finder for " + colName,
		})

		// findOneByColumnName
		index.AddVirtualMember(repoFQN, &symbols.Symbol{
			Name:       "findOneBy" + ucName,
			FQN:        repoFQN + "::findOneBy" + ucName,
			Kind:       symbols.KindMethod,
			URI:        repoSym.URI,
			Visibility: "public",
			ReturnType: "?" + entityFQN,
			Params:     []symbols.ParamInfo{{Name: "$value", Type: m.Type}},
			IsVirtual:  true,
			DocComment: "Magic finder for " + colName,
		})

		// countByColumnName
		index.AddVirtualMember(repoFQN, &symbols.Symbol{
			Name:       "countBy" + ucName,
			FQN:        repoFQN + "::countBy" + ucName,
			Kind:       symbols.KindMethod,
			URI:        repoSym.URI,
			Visibility: "public",
			ReturnType: "int",
			Params:     []symbols.ParamInfo{{Name: "$value", Type: m.Type}},
			IsVirtual:  true,
			DocComment: "Magic counter for " + colName,
		})
	}
}

// --- XML Mapping Fallback ---

// analyzeDoctrineXML looks for XML mapping files in config/doctrine/ and parses them.
func analyzeDoctrineXML(index *symbols.Index, entitySym *symbols.Symbol, rootPath, entityFQN string, cls *parser.ClassNode, resolve func(string) string) {
	if rootPath == "" {
		return
	}

	// Look for XML mapping files
	xmlPaths := []string{
		filepath.Join(rootPath, "config", "doctrine"),
		filepath.Join(rootPath, "config", "packages", "doctrine"),
	}

	for _, dir := range xmlPaths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".orm.xml") {
				continue
			}
			xmlPath := filepath.Join(dir, entry.Name())
			parseDoctrineXMLMapping(index, entitySym, entityFQN, xmlPath, resolve)
		}
	}
}

// XML mapping structures
type doctrineMappingXML struct {
	XMLName xml.Name           `xml:"doctrine-mapping"`
	Entity  *doctrineEntityXML `xml:"entity"`
}

type doctrineEntityXML struct {
	Name       string                `xml:"name,attr"`
	Table      string                `xml:"table,attr"`
	Fields     []doctrineFieldXML    `xml:"field"`
	Id         []doctrineFieldXML    `xml:"id"`
	OneToOne   []doctrineRelationXML `xml:"one-to-one"`
	OneToMany  []doctrineRelationXML `xml:"one-to-many"`
	ManyToOne  []doctrineRelationXML `xml:"many-to-one"`
	ManyToMany []doctrineRelationXML `xml:"many-to-many"`
}

type doctrineFieldXML struct {
	Name   string `xml:"name,attr"`
	Type   string `xml:"type,attr"`
	Column string `xml:"column,attr"`
}

type doctrineRelationXML struct {
	Field        string `xml:"field,attr"`
	TargetEntity string `xml:"target-entity,attr"`
	MappedBy     string `xml:"mapped-by,attr"`
}

func parseDoctrineXMLMapping(index *symbols.Index, entitySym *symbols.Symbol, entityFQN, xmlPath string, resolve func(string) string) {
	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return
	}

	var mapping doctrineMappingXML
	if err := xml.Unmarshal(data, &mapping); err != nil {
		return
	}

	if mapping.Entity == nil || mapping.Entity.Name != entityFQN {
		return
	}

	entity := mapping.Entity
	for _, field := range entity.Id {
		mapDoctrineXMLProperty(index, entitySym, field.Name, doctrineXMLFieldType(field), "Doctrine ID column", true)
	}
	for _, field := range entity.Fields {
		mapDoctrineXMLProperty(index, entitySym, field.Name, doctrineXMLFieldType(field), "Doctrine column", false)
	}
	for _, rel := range entity.OneToOne {
		mapDoctrineXMLProperty(index, entitySym, rel.Field, "?"+resolve(rel.TargetEntity), "OneToOne relation", false)
	}
	for _, rel := range entity.ManyToOne {
		mapDoctrineXMLProperty(index, entitySym, rel.Field, "?"+resolve(rel.TargetEntity), "ManyToOne relation", false)
	}
	for _, rel := range entity.OneToMany {
		mapDoctrineXMLProperty(index, entitySym, rel.Field, doctrineToManyType(resolve(rel.TargetEntity)), "OneToMany relation", false)
	}
	for _, rel := range entity.ManyToMany {
		mapDoctrineXMLProperty(index, entitySym, rel.Field, doctrineToManyType(resolve(rel.TargetEntity)), "ManyToMany relation", false)
	}
}

func doctrineXMLFieldType(field doctrineFieldXML) string {
	if phpType, ok := doctrineTypeMap[field.Type]; ok {
		return phpType
	}
	return "mixed"
}

func mapDoctrineXMLProperty(index *symbols.Index, entitySym *symbols.Symbol, field, phpType, doc string, readonly bool) {
	propName := "$" + field
	if index.RefineProperty(entitySym.FQN+"::"+propName, phpType, readonly) {
		return
	}
	index.AddVirtualMember(entitySym.FQN, &symbols.Symbol{
		Name:       propName,
		FQN:        entitySym.FQN + "::" + propName,
		Kind:       symbols.KindProperty,
		URI:        entitySym.URI,
		Visibility: "private",
		Type:       phpType,
		IsVirtual:  true,
		IsReadonly: readonly,
		DocComment: doc,
	})
}
