//go:build !wasip1

package models

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	// Database drivers
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"github.com/Tusk-PHP/lsp/internal/config"
)

// scanLiveSchema connects to the configured database and returns a normalized
// live schema snapshot. It is the native-only seam for ScanSchema; the wasip1
// build provides a stub that reports the feature unavailable.
func scanLiveSchema(dbCfg *config.DatabaseConfig, timeout time.Duration, cache *SchemaCache, logger *log.Logger) (*Schema, error) {
	db, err := openDatabase(dbCfg, timeout)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if logger != nil {
		logger.Printf("Connected to %s database: %s", dbCfg.Driver, dbCfg.Database)
	}
	schema, scanErr := scanSchemaFromDB(db, dbCfg, cache, timeout)
	if scanErr != nil {
		return nil, scanErr
	}
	schema.Source = SchemaSourceLive
	schema.Connected = true
	return schema, nil
}

func openDatabase(cfg *config.DatabaseConfig, timeout time.Duration) (*sql.DB, error) {
	var dsn, driverName string

	switch cfg.Driver {
	case "mysql":
		driverName = "mysql"
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?timeout=%s",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
			timeout.String())
	case "pgsql":
		driverName = "postgres"
		secs := int(timeout.Seconds())
		if secs < 1 {
			secs = 1
		}
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=%d",
			cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database, secs)
	case "sqlite":
		driverName = "sqlite"
		dsn = cfg.Database
	default:
		return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}

	// Verify connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	// Single connection — we just query schema then close
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)

	return db, nil
}

func queryColumns(db *sql.DB, dbName, tableName string, timeout time.Duration) ([]SchemaColumn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_TYPE
		 FROM INFORMATION_SCHEMA.COLUMNS
		 WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		 ORDER BY ORDINAL_POSITION`, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []SchemaColumn
	for rows.Next() {
		var c SchemaColumn
		var nullable string
		if err := rows.Scan(&c.Name, &c.DataType, &nullable, &c.ColumnType); err != nil {
			continue
		}
		c.IsNullable = nullable == "YES"
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// queryColumnsPostgres uses PostgreSQL's information_schema with $1/$2 params.
func queryColumnsPostgres(db *sql.DB, dbName, tableName string, timeout time.Duration) ([]SchemaColumn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable, udt_name
		 FROM information_schema.columns
		 WHERE table_catalog = $1 AND table_name = $2
		 ORDER BY ordinal_position`, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []SchemaColumn
	for rows.Next() {
		var c SchemaColumn
		var nullable string
		if err := rows.Scan(&c.Name, &c.DataType, &nullable, &c.ColumnType); err != nil {
			continue
		}
		c.IsNullable = strings.ToUpper(nullable) == "YES"
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// queryColumnsSQLite uses PRAGMA to get column info.
func queryColumnsSQLite(db *sql.DB, tableName string, timeout time.Duration) ([]SchemaColumn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []SchemaColumn
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &dflt, &pk); err != nil {
			continue
		}
		cols = append(cols, SchemaColumn{
			Name:       name,
			DataType:   strings.ToLower(dataType),
			IsNullable: notNull == 0 && pk == 0,
			ColumnType: dataType,
		})
	}
	return cols, rows.Err()
}

func queryTableNames(db *sql.DB, cfg *config.DatabaseConfig, timeout time.Duration) ([]string, error) {
	switch cfg.Driver {
	case "mysql":
		return queryTableNamesMySQL(db, cfg.Database, timeout)
	case "pgsql":
		return queryTableNamesPostgres(db, cfg.Database, timeout)
	case "sqlite":
		return queryTableNamesSQLite(db, timeout)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
	}
}

func queryTableNamesMySQL(db *sql.DB, dbName string, timeout time.Duration) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT TABLE_NAME
		 FROM INFORMATION_SCHEMA.TABLES
		 WHERE TABLE_SCHEMA = ?
		 ORDER BY TABLE_NAME`, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			continue
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func queryTableNamesPostgres(db *sql.DB, dbName string, timeout time.Duration) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT table_name
		 FROM information_schema.tables
		 WHERE table_catalog = $1 AND table_schema = 'public'
		 ORDER BY table_name`, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			continue
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func queryTableNamesSQLite(db *sql.DB, timeout time.Duration) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT name
		 FROM sqlite_master
		 WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			continue
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func getTableColumns(db *sql.DB, cfg *config.DatabaseConfig, tableName string, cache *SchemaCache, timeout time.Duration) []SchemaColumn {
	if cache != nil {
		if cols, ok := cache.Get(tableName); ok {
			return cols
		}
	}

	var cols []SchemaColumn
	var err error
	switch cfg.Driver {
	case "mysql":
		cols, err = queryColumns(db, cfg.Database, tableName, timeout)
	case "pgsql":
		cols, err = queryColumnsPostgres(db, cfg.Database, tableName, timeout)
	case "sqlite":
		cols, err = queryColumnsSQLite(db, tableName, timeout)
	}

	if err != nil || cols == nil {
		return nil
	}

	if cache != nil {
		cache.Set(tableName, cols)
	}
	return cols
}

func scanSchemaFromDB(db *sql.DB, cfg *config.DatabaseConfig, cache *SchemaCache, timeout time.Duration) (*Schema, error) {
	tables, err := queryTableNames(db, cfg, timeout)
	if err != nil {
		return nil, err
	}

	schema := &Schema{
		Source:     SchemaSourceLive,
		Connection: cfg.Driver,
		Database:   cfg.Database,
		Connected:  true,
		Tables:     make([]SchemaTable, 0, len(tables)),
	}

	for _, table := range tables {
		cols := getTableColumns(db, cfg, table, cache, timeout)
		if cols == nil {
			continue
		}
		schema.Tables = append(schema.Tables, SchemaTable{
			Name:    table,
			Columns: cols,
		})
	}

	return schema, nil
}
