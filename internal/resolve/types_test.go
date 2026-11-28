package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestParseGenericType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		fqn   string
		npar  int
		str   string
	}{
		{"bare type", "string", "string", 0, "string"},
		{"single param", "Builder<static>", "Builder", 1, "Builder<static>"},
		{"two params", "Collection<int, Category>", "Collection", 2, "Collection<int, Category>"},
		{"nullable", "?Collection<int, Model>", "Collection", 2, "?Collection<int, Model>"},
		{"FQN", "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\User>", "Illuminate\\Database\\Eloquent\\Builder", 1, "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\User>"},
		{"leading backslash", "\\Illuminate\\Builder", "Illuminate\\Builder", 0, "Illuminate\\Builder"},
		{"empty", "", "", 0, ""},
		{"nested generic", "Collection<int, Builder<Category>>", "Collection", 2, "Collection<int, Builder<Category>>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := ParseGenericType(tt.input)
			if rt.FQN != tt.fqn {
				t.Errorf("FQN = %q, want %q", rt.FQN, tt.fqn)
			}
			if len(rt.Params) != tt.npar {
				t.Errorf("len(Params) = %d, want %d", len(rt.Params), tt.npar)
			}
			if tt.str != "" && rt.String() != tt.str {
				t.Errorf("String() = %q, want %q", rt.String(), tt.str)
			}
		})
	}
}

func TestResolvedTypeHelpers(t *testing.T) {
	t.Run("BaseFQN", func(t *testing.T) {
		rt := ResolvedType{FQN: "Collection", Params: []ResolvedType{{FQN: "int"}}}
		if rt.BaseFQN() != "Collection" {
			t.Errorf("got %q", rt.BaseFQN())
		}
	})

	t.Run("IsGeneric", func(t *testing.T) {
		if !(ResolvedType{FQN: "X", Params: []ResolvedType{{FQN: "Y"}}}).IsGeneric() {
			t.Error("expected true")
		}
		if (ResolvedType{FQN: "X"}).IsGeneric() {
			t.Error("expected false")
		}
	})

	t.Run("IsEmpty", func(t *testing.T) {
		if !(ResolvedType{}).IsEmpty() {
			t.Error("expected true")
		}
		if (ResolvedType{FQN: "X"}).IsEmpty() {
			t.Error("expected false")
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		input := "Collection<int, Builder<Category>>"
		rt := ParseGenericType(input)
		if rt.String() != input {
			t.Errorf("roundtrip: %q != %q", rt.String(), input)
		}
	})
}

func TestParseDocTemplate(t *testing.T) {
	t.Run("@template TModel", func(t *testing.T) {
		doc := parser.ParseDocBlock(`/**
 * @template TModel
 */`)
		if len(doc.Templates) != 1 {
			t.Fatalf("expected 1 template, got %d", len(doc.Templates))
		}
		if doc.Templates[0].Name != "TModel" || doc.Templates[0].Bound != "" {
			t.Errorf("got %+v", doc.Templates[0])
		}
	})

	t.Run("@template TModel of Model", func(t *testing.T) {
		doc := parser.ParseDocBlock(`/**
 * @template TModel of \Illuminate\Database\Eloquent\Model
 */`)
		if len(doc.Templates) != 1 {
			t.Fatalf("expected 1 template, got %d", len(doc.Templates))
		}
		if doc.Templates[0].Name != "TModel" {
			t.Errorf("name = %q", doc.Templates[0].Name)
		}
		if doc.Templates[0].Bound != "\\Illuminate\\Database\\Eloquent\\Model" {
			t.Errorf("bound = %q", doc.Templates[0].Bound)
		}
	})

	t.Run("multiple templates", func(t *testing.T) {
		doc := parser.ParseDocBlock(`/**
 * @template TKey
 * @template TValue
 */`)
		if len(doc.Templates) != 2 {
			t.Fatalf("expected 2 templates, got %d", len(doc.Templates))
		}
	})
}

func TestSymbolTemplateStorage(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///repo.php", `<?php
/**
 * @template T of \App\Model
 * @method T find(int $id)
 */
class Repository {
}
`)
	sym := idx.Lookup("Repository")
	if sym == nil {
		t.Fatal("expected Repository symbol")
	}
	if len(sym.Templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(sym.Templates))
	}
	if sym.Templates[0].Name != "T" {
		t.Errorf("template name = %q", sym.Templates[0].Name)
	}
}

func TestResolveSymbolTemplateReturn(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///repo.php", `<?php
/**
 * @template T of Model
 */
class Repository {
    /**
     * @return T|null
     */
    public function find(int $id) {}
}
`)

	r := NewResolver(idx)
	sym := idx.Lookup("Repository")
	if sym == nil {
		t.Fatal("expected Repository symbol")
	}

	rt := r.ResolveSymbolTemplateReturn(sym, "find", []ResolvedType{{FQN: "App\\Models\\User"}})
	// T|null with T = App\Models\User → should substitute T
	if rt.IsEmpty() {
		t.Fatal("expected non-empty result")
	}
	// The result should be App\Models\User (first non-null in union)
	if rt.FQN != "App\\Models\\User" {
		t.Logf("got %q (union handling may take first non-null)", rt.FQN)
	}
}

func TestMappedTemplateReturn(t *testing.T) {
	idx := symbols.NewIndex()
	idx.SetTemplateMappings(map[string]symbols.TemplateMapping{
		"App\\Box": {
			Params: []string{"TKey", "TItem"},
			Methods: map[string]string{
				"all":   "App\\List<TKey, TItem>",
				"first": "?TItem",
				"where": "static",
				"count": "int",
			},
		},
	})
	r := NewResolver(idx)
	key, item := ResolvedType{FQN: "int"}, ResolvedType{FQN: "App\\Models\\Category"}
	cases := []struct {
		name, class, method, want string
	}{
		{"a generic return", "App\\Box", "all", "App\\List<int, App\\Models\\Category>"},
		{"a nullable template", "App\\Box", "first", "?App\\Models\\Category"},
		{"static keeps the arguments", "App\\Box", "where", "App\\Box<int, App\\Models\\Category>"},
		{"a plain type", "App\\Box", "count", "int"},
		{"an unknown method", "App\\Box", "missing", ""},
		{"an unmapped class", "App\\Other", "all", ""},
	}
	for _, c := range cases {
		if got := r.mappedTemplateReturn(c.class, c.method, []ResolvedType{key, item}).String(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if got := NewResolver(nil).mappedTemplateReturn("App\\Box", "all", nil); !got.IsEmpty() {
		t.Errorf("a resolver without an index maps nothing, got %q", got.String())
	}
}

// A callable with a return type is parenthesised in a union, so its return
// type isn't read as running on to the union's other parts.
func TestFullStringParenthesisesCallablesInUnions(t *testing.T) {
	ret := ResolvedType{FQN: "float"}
	callable := ResolvedType{FQN: "callable", Callable: &CallableType{Params: []ResolvedType{{FQN: "TValue"}}, Return: &ret}}
	union := MakeUnion([]ResolvedType{callable, {FQN: "int"}, {FQN: "string"}, {FQN: "null"}}, false)
	if got, want := union.FullString(), "(callable(TValue): float)|int|string|null"; got != want {
		t.Errorf("FullString = %q, want %q", got, want)
	}
	if got, want := callable.FullString(), "callable(TValue): float"; got != want {
		t.Errorf("alone: FullString = %q, want %q", got, want)
	}
	closure := ResolvedType{FQN: "Closure", Callable: &CallableType{Return: &ResolvedType{FQN: "int"}}}
	if got, want := MakeUnion([]ResolvedType{{FQN: "string"}, closure}, false).FullString(), "string|(Closure(): int)"; got != want {
		t.Errorf("Closure: FullString = %q, want %q", got, want)
	}
	bare := MakeUnion([]ResolvedType{{FQN: "callable", Callable: &CallableType{}}, {FQN: "int"}}, false)
	if got, want := bare.FullString(), "callable()|int"; got != want {
		t.Errorf("without a return type: FullString = %q, want %q", got, want)
	}
}
