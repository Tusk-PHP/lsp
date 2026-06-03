package models

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// defaultConnectTimeout is the fallback when no config is provided.
const defaultConnectTimeout = 5 * time.Second

// SchemaColumn represents a single column from the database schema.
type SchemaColumn struct {
	Name       string `json:"name"`
	DataType   string `json:"dataType"`
	IsNullable bool   `json:"isNullable"`
	ColumnType string `json:"columnType"` // full type, e.g. "tinyint(1)"
}

// SchemaTable represents a database table and its columns.
type SchemaTable struct {
	Name    string         `json:"name"`
	Columns []SchemaColumn `json:"columns"`
}

// Schema is a normalized read model of the live database schema.
type Schema struct {
	Source     string        `json:"source"`
	Connection string        `json:"connection"`
	Database   string        `json:"database"`
	Connected  bool          `json:"connected"`
	Caveat     string        `json:"caveat,omitempty"`
	Tables     []SchemaTable `json:"tables"`
}

const (
	SchemaSourceLive       = "live"
	SchemaSourceMigrations = "migrations"
	SchemaSourceNone       = "none"
)

// SchemaCache holds cached schema results per table.
type SchemaCache struct {
	mu     sync.RWMutex
	tables map[string][]SchemaColumn // tableName → columns
}

func NewSchemaCache() *SchemaCache {
	return &SchemaCache{tables: make(map[string][]SchemaColumn)}
}

func (sc *SchemaCache) Get(table string) ([]SchemaColumn, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	cols, ok := sc.tables[table]
	return cols, ok
}

func (sc *SchemaCache) Set(table string, cols []SchemaColumn) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.tables[table] = cols
}

func (sc *SchemaCache) Invalidate() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.tables = make(map[string][]SchemaColumn)
}

// SQL type → PHP type mapping.
var sqlTypeMap = map[string]string{
	"varchar":    "string",
	"char":       "string",
	"text":       "string",
	"tinytext":   "string",
	"mediumtext": "string",
	"longtext":   "string",
	"enum":       "string",
	"set":        "string",
	"int":        "int",
	"integer":    "int",
	"bigint":     "int",
	"smallint":   "int",
	"tinyint":    "int",
	"mediumint":  "int",
	"serial":     "int",
	"bigserial":  "int",
	"decimal":    "string",
	"numeric":    "string",
	"float":      "float",
	"double":     "float",
	"real":       "float",
	"datetime":   "\\DateTimeInterface",
	"timestamp":  "\\DateTimeInterface",
	"date":       "\\DateTimeInterface",
	"time":       "string",
	"json":       "array",
	"jsonb":      "array",
	"blob":       "string",
	"binary":     "string",
	"varbinary":  "string",
	"bytea":      "string",
	"uuid":       "string",
	"boolean":    "bool",
	"bool":       "bool",
}

// tinyint(1) detection pattern
var tinyint1Re = regexp.MustCompile(`(?i)tinyint\(\s*1\s*\)`)

// mapSQLTypeToPhp converts a SQL column type to PHP type.
func mapSQLTypeToPhp(dataType, columnType string) string {
	dt := strings.ToLower(dataType)
	// tinyint(1) is a boolean in PHP
	if dt == "tinyint" && tinyint1Re.MatchString(columnType) {
		return "bool"
	}
	if phpType, ok := sqlTypeMap[dt]; ok {
		return phpType
	}
	return "mixed"
}

// AnalyzeDatabaseSchema connects to the project's database and injects virtual
// properties for model/entity columns discovered from the schema.
func AnalyzeDatabaseSchema(index *symbols.Index, rootPath, framework string, cfg *config.Config, logger *log.Logger, cache *SchemaCache) {
	schema, err := ScanSchema(rootPath, framework, cfg, logger, cache)
	if err != nil || schema == nil {
		return
	}

	InjectDatabaseSchema(index, rootPath, framework, schema)
}

// InjectDatabaseSchema injects virtual members from a normalized schema into
// framework model/entity symbols. It does not perform any database I/O.
func InjectDatabaseSchema(index *symbols.Index, rootPath, framework string, schema *Schema) {
	if schema == nil {
		return
	}

	switch framework {
	case "laravel":
		injectLaravelSchemaProperties(index, schema, rootPath)
	case "symfony":
		injectSymfonySchemaProperties(index, schema, rootPath)
	}
}

// ScanSchema connects to the project's configured database and returns a
// normalized schema snapshot without mutating the symbol index.
func ScanSchema(rootPath, framework string, cfg *config.Config, logger *log.Logger, cache *SchemaCache) (*Schema, error) {
	if cfg != nil && !cfg.IsDatabaseEnabled() {
		return &Schema{Source: SchemaSourceNone, Caveat: "database introspection disabled"}, nil
	}

	dbCfg := resolveDatabaseConfig(rootPath, framework)
	mode := "auto"
	if cfg != nil {
		mode = cfg.DatabaseSourceMode()
	}

	timeout := defaultConnectTimeout
	if cfg != nil {
		timeout = cfg.DatabaseConnectTimeout()
	}

	base := &Schema{Source: SchemaSourceNone}
	if dbCfg != nil {
		base.Connection = dbCfg.Driver
		base.Database = dbCfg.Database
	}

	if mode != "migrations" && dbCfg != nil && dbCfg.Database != "" {
		schema, err := scanLiveSchema(dbCfg, timeout, cache, logger)
		if err == nil {
			return schema, nil
		}
		if logger != nil {
			logger.Printf("Database connection failed (will use fallback sources): %v", err)
		}
		if mode == "live" {
			base.Caveat = "live database unavailable"
			return base, nil
		}
	}

	if framework == "laravel" && mode != "live" {
		if schema := ScanMigrationSchema(rootPath); schema != nil {
			schema.Connection = base.Connection
			schema.Database = base.Database
			return schema, nil
		}
	}
	if base.Caveat == "" {
		base.Caveat = "no live database or migration-derived schema available"
	}
	return base, nil
}

func resolveDatabaseConfig(rootPath, framework string) *config.DatabaseConfig {
	switch framework {
	case "laravel":
		return config.ParseLaravelDatabaseConfig(rootPath)
	case "symfony":
		return config.ParseSymfonyDatabaseConfig(rootPath)
	default:
		return nil
	}
}

func (s *Schema) Table(name string) *SchemaTable {
	if s == nil || name == "" {
		return nil
	}
	for i := range s.Tables {
		if s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	return nil
}

func (s *Schema) TableNames() []string {
	if s == nil {
		return nil
	}
	names := make([]string, 0, len(s.Tables))
	for _, table := range s.Tables {
		names = append(names, table.Name)
	}
	return names
}

// injectLaravelSchemaProperties resolves table names for Laravel models and injects columns.
func injectLaravelSchemaProperties(index *symbols.Index, schema *Schema, rootPath string) {
	models := index.GetDescendants("Illuminate\\Database\\Eloquent\\Model")
	for _, model := range models {
		tableName := ResolveModelTableName(index, model, rootPath)
		if tableName == "" {
			continue
		}

		table := schema.Table(tableName)
		if table == nil {
			continue
		}
		injectColumnsAsProperties(index, model, table.Columns)
	}
}

// injectSymfonySchemaProperties resolves table names for Doctrine entities and injects columns.
func injectSymfonySchemaProperties(index *symbols.Index, schema *Schema, rootPath string) {
	uris := index.GetAllFileURIs()
	for _, uri := range uris {
		path := symbols.URIToPath(uri)
		content, err := readFileQuick(path)
		if err != nil || !strings.Contains(content, "ORM\\Entity") {
			continue
		}

		file := parser.ParseFile(content)
		if file == nil {
			continue
		}

		for _, cls := range file.Classes {
			fqn := file.Namespace + "\\" + cls.Name
			if file.Namespace == "" {
				fqn = cls.Name
			}
			sym := index.Lookup(fqn)
			if sym == nil {
				continue
			}

			tableName := resolveDoctrineTableName(content, &cls, fqn)
			if tableName == "" {
				continue
			}

			table := schema.Table(tableName)
			if table == nil {
				continue
			}
			injectColumnsAsProperties(index, sym, table.Columns)
		}
	}
}

func injectColumnsAsProperties(index *symbols.Index, classSym *symbols.Symbol, cols []SchemaColumn) {
	for _, col := range cols {
		propName := "$" + col.Name
		phpType := mapSQLTypeToPhp(col.DataType, col.ColumnType)
		if col.IsNullable && phpType != "mixed" {
			phpType = "?" + phpType
		}

		existing := index.Lookup(classSym.FQN + "::" + propName)
		if existing != nil {
			// Don't overwrite — other sources have priority
			continue
		}

		index.AddVirtualMember(classSym.FQN, &symbols.Symbol{
			Name:       propName,
			FQN:        classSym.FQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        classSym.URI,
			Visibility: "public",
			Type:       phpType,
			IsVirtual:  true,
			DocComment: fmt.Sprintf("(database column) %s — %s", col.DataType, col.ColumnType),
		})
	}
}

// ResolveModelTableName determines the database table name for a Laravel Eloquent model.
func ResolveModelTableName(index *symbols.Index, model *symbols.Symbol, rootPath string) string {
	// Check for explicit $table property
	tableProp := index.Lookup(model.FQN + "::$table")
	if tableProp != nil && tableProp.Value != "" {
		return strings.Trim(tableProp.Value, "'\"")
	}

	// Read source file and look for $table = 'name'
	path := symbols.URIToPath(model.URI)
	content, err := readFileQuick(path)
	if err == nil {
		if m := tablePropertyRe.FindStringSubmatch(content); m != nil {
			return m[1]
		}
	}

	// Convention: snake_case plural of class name
	return pluralize(toSnakeCase(model.Name))
}

// resolveDoctrineTableName determines the table name for a Doctrine entity.
func resolveDoctrineTableName(source string, cls *parser.ClassNode, fqn string) string {
	// Check #[ORM\Table(name: 'tablename')]
	if m := ormTableRe.FindStringSubmatch(source); m != nil {
		if nm := nameArgRe.FindStringSubmatch(m[1]); nm != nil {
			return nm[1]
		}
	}
	// Convention: snake_case of class name
	return toSnakeCase(cls.Name)
}

var tablePropertyRe = regexp.MustCompile(`\$table\s*=\s*['"]([^'"]+)['"]`)

// toSnakeCase converts CamelCase to snake_case.
func toSnakeCase(s string) string {
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

// pluralize applies simple English pluralization rules.
func pluralize(s string) string {
	if s == "" {
		return s
	}
	if strings.HasSuffix(s, "y") && len(s) > 1 && !isVowel(s[len(s)-2]) {
		return s[:len(s)-1] + "ies"
	}
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "sh") ||
		strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "x") ||
		strings.HasSuffix(s, "z") {
		return s + "es"
	}
	return s + "s"
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

func readFileQuick(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
