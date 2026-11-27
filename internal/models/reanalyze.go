package models

import (
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// AnalyzeModelFile re-runs model analysis for the models declared in one file
// after it was re-indexed. Re-indexing a file replaces its classes, which
// drops the relations, accessors, columns and builder methods the workspace's
// model analysis injected into them. schema and migrationColumns are what
// that analysis read; nil when it had none. Only uri's classes are touched,
// and source is the file's current text. Running it again over a file that
// was not re-indexed refreshes what changed elsewhere, such as the type of a
// cast class. It returns the classes the file's models' attribute cast types
// were read from.
func AnalyzeModelFile(index *symbols.Index, rootPath, framework, uri, source string, schema *Schema, migrationColumns map[string]map[string]*MigrationColumn) []string {
	switch framework {
	case "laravel":
		return analyzeEloquentFile(index, uri, source, schema, migrationColumns)
	case "symfony":
		analyzeDoctrineSource(index, rootPath, source)
		if schema != nil {
			injectDoctrineSchemaSource(index, schema, source)
		}
	}
	return nil
}

func analyzeEloquentFile(index *symbols.Index, uri, source string, schema *Schema, migrationColumns map[string]map[string]*MigrationColumn) (dependsOn []string) {
	var models []*symbols.Symbol
	for _, sym := range index.GetFileSymbols(uri) {
		if sym.Kind == symbols.KindClass && isEloquentModel(index, sym.FQN) {
			models = append(models, sym)
		}
	}
	if len(models) == 0 {
		return nil
	}
	parsed := newModelSource(source, index.GrammarOf(uri))
	for _, model := range models {
		injectEloquentStaticMethods(index, model)
		dependsOn = append(dependsOn, analyzeModelSource(index, model, parsed)...)
		// The same order as the workspace analysis: the schema's columns,
		// then those only migrations know.
		table := ResolveModelTableName(index, model)
		if schema != nil {
			if t := schema.Table(table); t != nil {
				injectColumnsAsProperties(index, model, t.Columns)
			}
		}
		if cols := migrationColumns[table]; cols != nil {
			injectMigrationColumnsInto(index, model, cols)
		}
	}
	return dependsOn
}

func isEloquentModel(index *symbols.Index, fqn string) bool {
	for _, parent := range index.GetInheritanceChain(fqn) {
		if parent == frameworklaravel.EloquentModel {
			return true
		}
	}
	return false
}
