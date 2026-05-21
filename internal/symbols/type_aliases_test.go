package symbols

import "testing"

func TestIndexStoresTypeAliasesAndImports(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///aliases.php", `<?php
namespace App;

class User {}

/**
 * @phpstan-type Payload array{id: int, user: User}
 */
class Source {}

/**
 * @phpstan-import-type Payload from Source as ImportedPayload
 */
class Consumer {}
`)

	source := idx.Lookup("App\\Source")
	if source == nil {
		t.Fatal("expected App\\Source symbol")
	}
	payload, ok := source.TypeAliases["Payload"]
	if !ok {
		t.Fatal("expected Payload alias on App\\Source")
	}
	if payload.Type != "array{id: int, user: App\\User}" {
		t.Fatalf("unexpected payload alias type %q", payload.Type)
	}

	consumer := idx.Lookup("App\\Consumer")
	if consumer == nil {
		t.Fatal("expected App\\Consumer symbol")
	}
	imported, ok := consumer.TypeAliases["ImportedPayload"]
	if !ok {
		t.Fatal("expected ImportedPayload alias on App\\Consumer")
	}
	if imported.Import == nil {
		t.Fatal("expected ImportedPayload to be stored as an import")
	}
	if imported.Import.FromFQN != "App\\Source" {
		t.Fatalf("unexpected import source %q", imported.Import.FromFQN)
	}
	if imported.Import.ImportedAs != "Payload" {
		t.Fatalf("unexpected imported alias name %q", imported.Import.ImportedAs)
	}
}
