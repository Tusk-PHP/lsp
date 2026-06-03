package models

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestSQLTypeMapping(t *testing.T) {
	tests := []struct {
		dataType   string
		columnType string
		expected   string
	}{
		{"varchar", "varchar(255)", "string"},
		{"int", "int(11)", "int"},
		{"bigint", "bigint(20)", "int"},
		{"tinyint", "tinyint(1)", "bool"},
		{"tinyint", "tinyint(4)", "int"},
		{"decimal", "decimal(10,2)", "string"},
		{"float", "float", "float"},
		{"double", "double", "float"},
		{"datetime", "datetime", "\\DateTimeInterface"},
		{"timestamp", "timestamp", "\\DateTimeInterface"},
		{"date", "date", "\\DateTimeInterface"},
		{"json", "json", "array"},
		{"text", "text", "string"},
		{"blob", "blob", "string"},
		{"boolean", "boolean", "bool"},
		{"uuid", "uuid", "string"},
	}
	for _, tt := range tests {
		got := mapSQLTypeToPhp(tt.dataType, tt.columnType)
		if got != tt.expected {
			t.Errorf("mapSQLTypeToPhp(%q, %q) = %q, want %q", tt.dataType, tt.columnType, got, tt.expected)
		}
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"user", "users"},
		{"category", "categories"},
		{"post", "posts"},
		{"bus", "buses"},
		{"match", "matches"},
		{"fox", "foxes"},
		{"quiz", "quizes"},
		{"photo", "photos"},
	}
	for _, tt := range tests {
		if got := pluralize(tt.input); got != tt.expected {
			t.Errorf("pluralize(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"User", "user"},
		{"BlogPost", "blog_post"},
		{"Category", "category"},
		{"UserProfile", "user_profile"},
	}
	for _, tt := range tests {
		if got := toSnakeCase(tt.input); got != tt.expected {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSQLiteSchemaIntrospection(t *testing.T) {
	// Create a SQLite database with a test table
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		email VARCHAR(255) NOT NULL,
		age INTEGER,
		is_active BOOLEAN DEFAULT 1,
		balance REAL,
		metadata TEXT,
		created_at DATETIME
	)`)
	if err != nil {
		t.Fatal(err)
	}

	// Query using our function
	cols, err := queryColumnsSQLite(db, "users", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if len(cols) != 8 {
		t.Fatalf("expected 8 columns, got %d", len(cols))
	}

	// Verify column details
	colMap := make(map[string]SchemaColumn)
	for _, c := range cols {
		colMap[c.Name] = c
	}

	t.Run("id column", func(t *testing.T) {
		c := colMap["id"]
		if c.DataType != "integer" {
			t.Errorf("expected type 'integer', got %q", c.DataType)
		}
		if c.IsNullable {
			t.Error("id should not be nullable (it's PK)")
		}
	})

	t.Run("name column", func(t *testing.T) {
		c := colMap["name"]
		if c.DataType != "text" {
			t.Errorf("expected type 'text', got %q", c.DataType)
		}
		if c.IsNullable {
			t.Error("name should not be nullable (NOT NULL)")
		}
	})

	t.Run("age nullable column", func(t *testing.T) {
		c := colMap["age"]
		if !c.IsNullable {
			t.Error("age should be nullable")
		}
	})
}

func TestFullSQLiteIntegration(t *testing.T) {
	// Create a SQLite database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "app.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		bio TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Write .env file
	envContent := "DB_CONNECTION=sqlite\nDB_DATABASE=" + dbPath + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a User model
	modelSource := `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class User extends Model {}
`
	modelPath := filepath.Join(tmpDir, "User.php")
	if err := os.WriteFile(modelPath, []byte(modelSource), 0644); err != nil {
		t.Fatal(err)
	}

	// Setup index
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {}
`)
	idx.IndexFile("file://"+modelPath, modelSource)

	// Run database analysis
	cache := NewSchemaCache()
	AnalyzeDatabaseSchema(idx, tmpDir, "laravel", nil, nil, cache)

	schema, err := ScanSchema(tmpDir, "laravel", nil, nil, cache)
	if err != nil {
		t.Fatalf("ScanSchema() error = %v", err)
	}
	if schema == nil {
		t.Fatal("expected schema from ScanSchema")
	}

	t.Run("database columns injected", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$name")
		if sym == nil {
			t.Fatal("expected virtual property '$name' from database")
		}
		if !sym.IsVirtual {
			t.Error("expected virtual")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string', got %q", sym.Type)
		}
	})

	t.Run("nullable column has nullable type", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$bio")
		if sym == nil {
			t.Fatal("expected virtual property '$bio' from database")
		}
		if sym.Type != "?string" {
			t.Errorf("expected type '?string', got %q", sym.Type)
		}
	})

	t.Run("id column injected", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$id")
		if sym == nil {
			t.Fatal("expected virtual property '$id' from database")
		}
		if sym.Type != "int" {
			t.Errorf("expected type 'int', got %q", sym.Type)
		}
	})

	t.Run("schema cached", func(t *testing.T) {
		cols, ok := cache.Get("users")
		if !ok {
			t.Fatal("expected cached columns for 'users'")
		}
		if len(cols) != 4 {
			t.Errorf("expected 4 cached columns, got %d", len(cols))
		}
	})

	t.Run("normalized schema returned", func(t *testing.T) {
		if schema.Connection != "sqlite" {
			t.Fatalf("expected sqlite connection, got %q", schema.Connection)
		}
		if schema.Source != SchemaSourceLive {
			t.Fatalf("expected live schema source, got %q", schema.Source)
		}
		if !schema.Connected {
			t.Fatal("expected connected live schema")
		}
		if schema.Database != dbPath {
			t.Fatalf("expected database path %q, got %q", dbPath, schema.Database)
		}
		if got := schema.Table("users"); got == nil {
			t.Fatal("expected users table in normalized schema")
		} else if len(got.Columns) != 4 {
			t.Fatalf("expected 4 columns in users table, got %d", len(got.Columns))
		}
	})
}

func TestDatabaseDisabled(t *testing.T) {
	idx := symbols.NewIndex()
	disabled := false
	cfg := &config.Config{DatabaseEnabled: &disabled}

	// Should return immediately without error
	AnalyzeDatabaseSchema(idx, "/tmp", "laravel", cfg, nil, NewSchemaCache())
}

func TestScanSchemaDisabled(t *testing.T) {
	disabled := false
	cfg := &config.Config{DatabaseEnabled: &disabled}

	schema, err := ScanSchema("/tmp", "laravel", cfg, nil, NewSchemaCache())
	if err != nil {
		t.Fatalf("ScanSchema() error = %v", err)
	}
	if schema == nil {
		t.Fatal("expected none schema when database is disabled")
	}
	if schema.Source != SchemaSourceNone {
		t.Fatalf("expected none source, got %q", schema.Source)
	}
}

func TestScanSchemaFallsBackToMigrations(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "laravel")
	cfg := &config.Config{Framework: "laravel", DatabaseSource: "migrations"}

	schema, err := ScanSchema(root, "laravel", cfg, nil, NewSchemaCache())
	if err != nil {
		t.Fatalf("ScanSchema() error = %v", err)
	}
	if schema == nil {
		t.Fatal("expected schema")
	}
	if schema.Source != SchemaSourceMigrations {
		t.Fatalf("expected migrations source, got %q", schema.Source)
	}
	if schema.Connected {
		t.Fatal("expected disconnected schema for migrations fallback")
	}
	if got := schema.Table("users"); got == nil {
		t.Fatal("expected users table from migrations")
	}
}
