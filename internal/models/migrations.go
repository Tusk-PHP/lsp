package models

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// MigrationColumn represents a column discovered from a migration file.
type MigrationColumn struct {
	Name     string
	Type     string // PHP type
	RawType  string
	Nullable bool
	Default  string
}

// Blueprint method → PHP type mapping.
var blueprintTypeMap = map[string]string{
	"string":                "string",
	"text":                  "string",
	"mediumText":            "string",
	"longText":              "string",
	"char":                  "string",
	"integer":               "int",
	"tinyInteger":           "int",
	"smallInteger":          "int",
	"mediumInteger":         "int",
	"bigInteger":            "int",
	"unsignedInteger":       "int",
	"unsignedTinyInteger":   "int",
	"unsignedSmallInteger":  "int",
	"unsignedMediumInteger": "int",
	"unsignedBigInteger":    "int",
	"boolean":               "bool",
	"decimal":               "string",
	"unsignedDecimal":       "string",
	"float":                 "float",
	"double":                "float",
	"date":                  "\\DateTimeInterface",
	"dateTime":              "\\DateTimeInterface",
	"dateTimeTz":            "\\DateTimeInterface",
	"timestamp":             "\\DateTimeInterface",
	"timestampTz":           "\\DateTimeInterface",
	"time":                  "string",
	"timeTz":                "string",
	"year":                  "int",
	"json":                  "array",
	"jsonb":                 "array",
	"binary":                "string",
	"uuid":                  "string",
	"ulid":                  "string",
	"ipAddress":             "string",
	"macAddress":            "string",
	"enum":                  "string",
	"set":                   "string",
	"foreignId":             "int",
	"foreignUlid":           "string",
	"foreignUuid":           "string",
}

// AnalyzeMigrations scans Laravel migration files and injects virtual properties
// for columns that don't already have a source from DB, IDE helper, or docblocks.
func AnalyzeMigrations(index *symbols.Index, rootPath string) {
	InjectMigrationColumns(index, rootPath, ScanMigrationColumns(rootPath))
}

// ScanMigrationColumns reads the columns of each table from the Laravel
// migration files under rootPath, by table name.
func ScanMigrationColumns(rootPath string) map[string]map[string]*MigrationColumn {
	migrationsDir := filepath.Join(rootPath, "database", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil
	}

	// Sort by filename (timestamped, so lexicographic order = chronological)
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".php") {
			migrationFiles = append(migrationFiles, filepath.Join(migrationsDir, entry.Name()))
		}
	}
	sort.Strings(migrationFiles)

	// Build table schemas from migrations
	tableColumns := make(map[string]map[string]*MigrationColumn)

	for _, path := range migrationFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parseMigrationFile(string(content), tableColumns)
	}
	return tableColumns
}

// InjectMigrationColumns adds the columns migrations give each model's table
// as virtual properties of the model.
func InjectMigrationColumns(index *symbols.Index, rootPath string, tableColumns map[string]map[string]*MigrationColumn) {
	if len(tableColumns) == 0 {
		return
	}
	// Map tables to models and inject virtual properties
	modelsByTable := buildModelTableMap(index, rootPath)

	for tableName, columns := range tableColumns {
		modelFQN, ok := modelsByTable[tableName]
		if !ok {
			continue
		}
		if modelSym := index.Lookup(modelFQN); modelSym != nil {
			injectMigrationColumnsInto(index, modelSym, columns)
		}
	}
}

// injectMigrationColumnsInto adds columns to model as virtual properties,
// leaving any property the model already has.
func injectMigrationColumnsInto(index *symbols.Index, modelSym *symbols.Symbol, columns map[string]*MigrationColumn) {
	modelFQN := modelSym.FQN
	for _, col := range columns {
		propName := "$" + col.Name
		// Skip if already discovered from another source
		if index.Lookup(modelFQN+"::"+propName) != nil {
			continue
		}

		phpType := col.Type
		if col.Nullable && phpType != "mixed" {
			phpType = "?" + phpType
		}

		docComment := "From migration"
		if col.Default != "" {
			docComment += ", default: " + col.Default
		}

		index.AddVirtualMember(modelFQN, &symbols.Symbol{
			Name:       propName,
			FQN:        modelFQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        modelSym.URI,
			Visibility: "public",
			Type:       phpType,
			IsVirtual:  true,
			DocComment: docComment,
		})
	}
}

func ScanMigrationSchema(rootPath string) *Schema {
	migrationsDir := filepath.Join(rootPath, "database", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil
	}

	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".php") {
			migrationFiles = append(migrationFiles, filepath.Join(migrationsDir, entry.Name()))
		}
	}
	sort.Strings(migrationFiles)

	tableColumns := make(map[string]map[string]*MigrationColumn)
	for _, path := range migrationFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parseMigrationFile(string(content), tableColumns)
	}
	if len(tableColumns) == 0 {
		return nil
	}

	schema := &Schema{
		Source:    SchemaSourceMigrations,
		Connected: false,
		Caveat:    "inferred from migration files; may not reflect the applied database state",
		Tables:    make([]SchemaTable, 0, len(tableColumns)),
	}

	var tableNames []string
	for name := range tableColumns {
		tableNames = append(tableNames, name)
	}
	sort.Strings(tableNames)

	for _, tableName := range tableNames {
		cols := tableColumns[tableName]
		table := SchemaTable{Name: tableName}
		var colNames []string
		for name := range cols {
			colNames = append(colNames, name)
		}
		sort.Strings(colNames)
		for _, colName := range colNames {
			col := cols[colName]
			dataType := col.RawType
			if dataType == "" {
				dataType = col.Type
			}
			table.Columns = append(table.Columns, SchemaColumn{
				Name:       col.Name,
				DataType:   dataType,
				IsNullable: col.Nullable,
				ColumnType: dataType,
			})
		}
		schema.Tables = append(schema.Tables, table)
	}
	return schema
}

// buildModelTableMap creates a mapping from table names to model FQNs.
func buildModelTableMap(index *symbols.Index, rootPath string) map[string]string {
	result := make(map[string]string)
	models := index.GetDescendants("Illuminate\\Database\\Eloquent\\Model")
	for _, model := range models {
		tableName := ResolveModelTableName(index, model, rootPath)
		if tableName != "" {
			result[tableName] = model.FQN
		}
	}
	return result
}
