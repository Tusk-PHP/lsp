package models

import (
	"os"
	"sort"
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Relation describes a directed relationship edge between two model/entity classes.
type Relation struct {
	SourceModel  string `json:"sourceModel"`  // FQN of the owning model/entity
	TargetModel  string `json:"targetModel"`  // resolved FQN of the related model/entity
	RelationType string `json:"relationType"` // PascalCase, e.g. "HasMany", "BelongsTo", "OneToMany"
	Cardinality  string `json:"cardinality"`  // "one" | "many"
	MethodName   string `json:"methodName"`   // method or property name that declares the relation
}

// ModelClasses enumerates the concrete model/entity symbols for the given framework.
//
// For "laravel" it returns every class that is a descendant of
// Illuminate\Database\Eloquent\Model (excluding the base itself).
// For "symfony" it scans all indexed PHP files for Doctrine ORM entity classes.
// Any other framework value returns nil.
//
// The returned slice is sorted by FQN and deduplicated.
func ModelClasses(index *symbols.Index, rootPath, framework string) []*symbols.Symbol {
	switch framework {
	case "laravel":
		return laravelModelClasses(index)
	case "symfony":
		return symfonyEntityClasses(index)
	default:
		return nil
	}
}

// ModelRelations re-derives relation edges directly from source, bypassing the
// post-analysis virtual properties (which collapse to-many targets to Collection).
//
// For "laravel" it mirrors the analyzeModel relation-detection logic.
// For "symfony" it scans Doctrine ORM relation attributes on entity properties.
//
// The returned slice is sorted deterministically by (SourceModel, MethodName, TargetModel).
func ModelRelations(index *symbols.Index, rootPath, framework string) []Relation {
	switch framework {
	case "laravel":
		return laravelRelations(index)
	case "symfony":
		return symfonyRelations(index)
	default:
		return nil
	}
}

// --- Laravel / Eloquent ---

func laravelModelClasses(index *symbols.Index) []*symbols.Symbol {
	raw := index.GetDescendants(frameworklaravel.EloquentModel)
	result := make([]*symbols.Symbol, 0, len(raw))
	for _, sym := range raw {
		if sym.FQN == frameworklaravel.EloquentModel {
			continue
		}
		result = append(result, sym)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].FQN < result[j].FQN
	})
	return result
}

func laravelRelations(index *symbols.Index) []Relation {
	models := laravelModelClasses(index)
	var relations []Relation

	for _, model := range models {
		rels := extractEloquentRelations(model, index.GrammarOf(model.URI))
		relations = append(relations, rels...)
	}

	sortRelations(relations)
	return relations
}

// extractEloquentRelations mirrors the relation-detection portion of analyzeModel,
// but collects Relation structs instead of injecting virtual members.
func extractEloquentRelations(model *symbols.Symbol, grammars phpparser.GrammarRange) (result []Relation) {
	defer func() {
		// Panic-safe: skip the model on any unexpected error.
		if r := recover(); r != nil {
			result = nil
		}
	}()

	path := symbols.URIToPath(model.URI)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	source := string(content)
	lines := strings.Split(source, "\n")

	file := parser.ParseFileIn(source, grammars)
	if file == nil {
		return nil
	}

	// Find the class node matching this model's FQN.
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
		return nil
	}

	resolve := func(name string) string {
		return resolveWithUses(name, file.Namespace, file.Uses)
	}

	for _, method := range classNode.Methods {
		returnShort := shortClassName(method.ReturnType.Name)

		var relType string
		if allRelationTypes[returnShort] {
			// Explicit relation return type.
			relType = returnShort
		} else if method.ReturnType.Name == "" || method.ReturnType.Name == "mixed" {
			// No explicit return type: check method body for a relation call.
			body := extractMethodBody(lines, method.StartLine, method.EndLine)
			if match := relationCallRe.FindStringSubmatch(body); match != nil {
				relType = ucFirst(match[1])
			}
		}

		if relType == "" {
			continue
		}

		// Extract the target class from the method body.
		body := extractMethodBody(lines, method.StartLine, method.EndLine)
		match := relationCallRe.FindStringSubmatch(body)
		if match == nil {
			continue
		}
		target := resolve(match[2])
		if target == "" {
			continue
		}

		cardinality := "one"
		if pluralRelations[relType] {
			cardinality = "many"
		}

		result = append(result, Relation{
			SourceModel:  model.FQN,
			TargetModel:  target,
			RelationType: relType,
			Cardinality:  cardinality,
			MethodName:   method.Name,
		})
	}

	return result
}

// --- Symfony / Doctrine ---

type indexedEntity struct {
	symbol *symbols.Symbol
	doctrineEntity
}

func symfonyEntities(index *symbols.Index) []indexedEntity {
	seen := make(map[string]bool)
	var result []indexedEntity
	for _, uri := range index.GetAllFileURIs() {
		content, err := os.ReadFile(symbols.URIToPath(uri))
		if err != nil || !strings.Contains(string(content), doctrineMapping[:len(doctrineMapping)-1]) {
			continue
		}
		for _, entity := range doctrineEntities(parser.ParseFileIn(string(content), index.GrammarOf(uri))) {
			sym := index.Lookup(entity.fqn)
			if sym == nil || seen[entity.fqn] {
				continue
			}
			seen[entity.fqn] = true
			result = append(result, indexedEntity{symbol: sym, doctrineEntity: entity})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].symbol.FQN < result[j].symbol.FQN })
	return result
}

func symfonyEntityClasses(index *symbols.Index) []*symbols.Symbol {
	entities := symfonyEntities(index)
	result := make([]*symbols.Symbol, len(entities))
	for i, entity := range entities {
		result[i] = entity.symbol
	}
	return result
}

func symfonyRelations(index *symbols.Index) []Relation {
	var relations []Relation
	for _, entity := range symfonyEntities(index) {
		for _, prop := range entity.class.Properties {
			relation, ok := entity.relation(prop)
			if !ok || relation.target == "" {
				continue
			}
			cardinality := "one"
			if doctrinePluralRelations[relation.kind] {
				cardinality = "many"
			}
			relations = append(relations, Relation{
				SourceModel:  entity.symbol.FQN,
				TargetModel:  relation.target,
				RelationType: relation.kind,
				Cardinality:  cardinality,
				MethodName:   strings.TrimPrefix(relation.name, "$"),
			})
		}
	}
	sortRelations(relations)
	return relations
}

// sortRelations sorts a slice of Relation deterministically by
// (SourceModel, MethodName, TargetModel).
func sortRelations(rels []Relation) {
	sort.Slice(rels, func(i, j int) bool {
		a, b := rels[i], rels[j]
		if a.SourceModel != b.SourceModel {
			return a.SourceModel < b.SourceModel
		}
		if a.MethodName != b.MethodName {
			return a.MethodName < b.MethodName
		}
		return a.TargetModel < b.TargetModel
	})
}
