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

func extractEloquentRelations(model *symbols.Symbol, grammars phpparser.GrammarRange) []Relation {
	content, err := os.ReadFile(symbols.URIToPath(model.URI))
	if err != nil {
		return nil
	}
	source := newModelSource(string(content), grammars)
	classNode, decls := source.class(model.FQN)
	if classNode == nil {
		return nil
	}
	var result []Relation
	for _, method := range classNode.Methods {
		relation, ok := source.relation(method, decls[method.Name])
		if !ok || relation.target == "" {
			continue
		}
		cardinality := "one"
		if frameworklaravel.IsPluralRelationType(relation.kind) {
			cardinality = "many"
		}
		result = append(result, Relation{
			SourceModel:  model.FQN,
			TargetModel:  relation.target,
			RelationType: relation.kind,
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
