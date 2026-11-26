package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestSymfonySchemaColumnsReachEntitiesReadFromFilesOnly(t *testing.T) {
	entity := func(class, table string) string {
		return "<?php\nnamespace App\\Entity;\n\nuse Doctrine\\ORM\\Mapping as ORM;\n\n#[ORM\\Entity]\n#[ORM\\Table(name: '" + table + "')]\nclass " + class + "\n{\n}\n"
	}
	path := filepath.Join(t.TempDir(), "src", "Entity", "User.php")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(entity("User", "users")), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.IndexFile(symbols.PathToURI(path), entity("User", "users"))
	idx.IndexFile("untitled:Draft.php", entity("Draft", "drafts"))
	schema := &Schema{Source: SchemaSourceLive, Tables: []SchemaTable{
		{Name: "users", Columns: []SchemaColumn{{Name: "email", DataType: "varchar"}}},
		{Name: "drafts", Columns: []SchemaColumn{{Name: "title", DataType: "varchar"}}},
	}}
	InjectDatabaseSchema(idx, "/", "symfony", schema)
	if !hasMember(idx, `App\Entity\User`, "$email") {
		t.Error("the entity read from its file gets its table's columns")
	}
	if hasMember(idx, `App\Entity\Draft`, "$title") {
		t.Error("a document that is not a file is not read from disk")
	}
}
