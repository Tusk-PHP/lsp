package models

import (
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// AnalyzeModelFile re-runs model analysis for the models declared in one file
// after it was re-indexed. Re-indexing a file replaces its classes, which
// drops the relations, accessors, columns and builder methods the workspace's
// model analysis injected into them. schema and migrationColumns are what
// that analysis read; nil when it had none. Only uri's classes are touched,
// and source is the file's current text.
func AnalyzeModelFile(index *symbols.Index, rootPath, framework, uri, source string, schema *Schema, migrationColumns map[string]map[string]*MigrationColumn) {
	switch framework {
	case "laravel":
		analyzeEloquentFile(index, uri, source, schema, migrationColumns)
	case "symfony":
		analyzeDoctrineSource(index, rootPath, source)
		if schema != nil {
			injectDoctrineSchemaSource(index, schema, source)
		}
	}
}

func analyzeEloquentFile(index *symbols.Index, uri, source string, schema *Schema, migrationColumns map[string]map[string]*MigrationColumn) {
	var models []*symbols.Symbol
	for _, sym := range index.GetFileSymbols(uri) {
		if sym.Kind == symbols.KindClass && isEloquentModel(index, sym.FQN) {
			models = append(models, sym)
		}
	}
	if len(models) == 0 {
		return
	}
	file := parser.ParseFileIn(source, index.GrammarOf(uri))
	if file == nil {
		return
	}
	for _, model := range models {
		injectEloquentStaticMethods(index, model)
		analyzeModelSource(index, model, source, file)
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
}

func isEloquentModel(index *symbols.Index, fqn string) bool {
	for _, parent := range index.GetInheritanceChain(fqn) {
		if parent == frameworklaravel.EloquentModel {
			return true
		}
	}
	return false
}
