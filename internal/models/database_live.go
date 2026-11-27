//go:build !wasip1

package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"modernc.org/sqlite"

	"github.com/Tusk-PHP/lsp/internal/config"
)

// scanLiveSchema connects to the configured database and returns a normalized
// live schema snapshot. It is the native-only seam for ScanSchema; the wasip1
// build provides a stub that reports the feature unavailable.
func scanLiveSchema(dbCfg *config.DatabaseConfig, timeout time.Duration, cache *SchemaCache, logger *log.Logger) (*Schema, error) {
	db, err := openDatabase(dbCfg, timeout)
	if err != nil {
		return nil, &liveSchemaError{step: "connect", err: err}
	}
	defer db.Close()
	if logger != nil {
		logger.Print("Connected to the project's database")
	}
	schema, scanErr := scanSchemaFromDB(db, dbCfg, cache, timeout)
	if scanErr != nil {
		return nil, &liveSchemaError{step: "read the schema", err: scanErr}
	}
	schema.Source = SchemaSourceLive
	schema.Connected = true
	return schema, nil
}

var errUnsupportedDriver = errors.New("unsupported database driver")

type liveSchemaError struct {
	step string
	err  error
}

func (e *liveSchemaError) Error() string {
	return "could not " + e.step + ": " + valueFreeReason(e.err)
}

func (e *liveSchemaError) Unwrap() error {
	return e.err
}

func valueFreeReason(err error) string {
	var mysqlErr *mysql.MySQLError
	var pqErr *pq.Error
	var sqliteErr *sqlite.Error
	var dnsErr *net.DNSError
	var netErr *net.OpError
	switch {
	case errors.Is(err, errUnsupportedDriver):
		return errUnsupportedDriver.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.As(err, &mysqlErr):
		return "MySQL error " + strconv.Itoa(int(mysqlErr.Number))
	case errors.As(err, &pqErr):
		return "PostgreSQL error " + string(pqErr.Code)
	case errors.As(err, &sqliteErr):
		return "SQLite error " + strconv.Itoa(sqliteErr.Code())
	case errors.As(err, &dnsErr):
		return "host not found"
	case errors.As(err, &netErr):
		if netErr.Timeout() {
			return "timed out"
		}
		return "network error on " + netErr.Op
	}
	return "driver error"
}

func openDatabase(cfg *config.DatabaseConfig, timeout time.Duration) (*sql.DB, error) {
	driverName, dsn, err := dataSource(cfg, timeout)
	if err != nil {
		return nil, err
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

func dataSource(cfg *config.DatabaseConfig, timeout time.Duration) (driverName, dsn string, err error) {
	switch cfg.Driver {
	case "mysql":
		mysqlCfg := mysql.NewConfig()
		mysqlCfg.User = cfg.Username
		mysqlCfg.Passwd = cfg.Password
		mysqlCfg.Net = "tcp"
		mysqlCfg.Addr = net.JoinHostPort(unbracketed(cfg.Host), cfg.Port)
		mysqlCfg.DBName = cfg.Database
		mysqlCfg.Timeout = timeout
		return "mysql", mysqlCfg.FormatDSN(), nil
	case "pgsql":
		return "postgres", postgresDSN(cfg, timeout), nil
	case "sqlite":
		return "sqlite", cfg.Database, nil
	default:
		return "", "", errUnsupportedDriver
	}
}

var libpqEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func postgresDSN(cfg *config.DatabaseConfig, timeout time.Duration) string {
	params := [...]struct{ key, value string }{
		{"host", unbracketed(cfg.Host)},
		{"port", cfg.Port},
		{"user", cfg.Username},
		{"password", cfg.Password},
		{"dbname", cfg.Database},
		{"sslmode", "disable"},
		{"connect_timeout", strconv.Itoa(max(int(timeout.Seconds()), 1))},
	}
	var b strings.Builder
	for i, p := range params {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(p.key)
		b.WriteString("='")
		libpqEscaper.WriteString(&b, p.value)
		b.WriteByte('\'')
	}
	return b.String()
}

func unbracketed(host string) string {
	if inner, ok := strings.CutPrefix(host, "["); ok {
		if inner, ok := strings.CutSuffix(inner, "]"); ok {
			return inner
		}
	}
	return host
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
		return nil, errUnsupportedDriver
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
