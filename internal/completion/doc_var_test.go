package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestExtractShapeFromDocParamsMatchesTheVariableByName(t *testing.T) {
	doc := "/**\n * @var array{id: int} $rows\n * @var array{name: string, age: int} $row\n */"
	fields := extractShapeFromDocParams(doc, "row")
	if len(fields) != 2 || fields[0].Key != "name" || fields[1].Key != "age" {
		t.Fatalf("fields = %+v, want name and age", fields)
	}
	if fields := extractShapeFromDocParams(doc, "other"); fields != nil {
		t.Fatalf("fields for $other = %+v, want none", fields)
	}
}

func TestDocblockVarTypeReadsTheWholeType(t *testing.T) {
	for doc, want := range map[string]string{
		"/** @var array{a: int, b: string} */":    "array{a: int, b: string}",
		"/** @var callable(int, string): void */": "callable(int, string): void",
		"/** @var Collection<int, User> items */": "Collection<int, User>",
		"/** no var here */":                      "",
	} {
		if got := docblockVarType(&symbols.Symbol{DocComment: doc}); got != want {
			t.Errorf("docblockVarType(%s) = %q, want %q", doc, got, want)
		}
	}
}
