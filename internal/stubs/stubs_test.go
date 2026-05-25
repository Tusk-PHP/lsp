package stubs

import (
	"reflect"
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
)

func TestBuiltinPHPReturnsDeterministicEntries(t *testing.T) {
	entriesA, err := BuiltinPHP()
	if err != nil {
		t.Fatalf("BuiltinPHP() error = %v", err)
	}

	entriesB, err := BuiltinPHP()
	if err != nil {
		t.Fatalf("BuiltinPHP() second call error = %v", err)
	}

	if !reflect.DeepEqual(entriesA, entriesB) {
		t.Fatal("BuiltinPHP() returned non-deterministic entries")
	}

	if len(entriesA) == 0 {
		t.Fatal("BuiltinPHP() returned no entries")
	}

	for i := 1; i < len(entriesA); i++ {
		if entriesA[i-1].Path > entriesA[i].Path {
			t.Fatalf("entries out of order: %q before %q", entriesA[i-1].Path, entriesA[i].Path)
		}
	}
}

func TestBuiltinPHPEntriesAreParseable(t *testing.T) {
	entries, err := BuiltinPHP()
	if err != nil {
		t.Fatalf("BuiltinPHP() error = %v", err)
	}

	for _, entry := range entries {
		file := parser.ParseFile(entry.Content)
		if file == nil {
			t.Fatalf("ParseFile(%q) returned nil", entry.Path)
		}
	}
}
