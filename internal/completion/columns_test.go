package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestExtractBuilderArgContext(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantOk   bool
		wantMeth string
		wantPart string
		wantQt   string
	}{
		{"static where single quote", "Category::where('", true, "where", "", "'"},
		{"static where partial", "Category::where('na", true, "where", "na", "'"},
		{"static where double quote", `Category::where("na`, true, "where", "na", "\""},
		{"static where no quote", "Category::where(", true, "where", "", ""},
		{"instance orderBy", "$q->orderBy('na", true, "orderBy", "na", "'"},
		{"chained where", "Category::query()->where('", true, "where", "", "'"},
		{"with relation", "Category::with('pro", true, "with", "pro", "'"},
		{"whereHas", "$cat->whereHas('", true, "whereHas", "", "'"},
		{"has", "Category::has('", true, "has", "", "'"},
		{"select", "Category::select('", true, "select", "", "'"},
		{"pluck", "$q->pluck('", true, "pluck", "", "'"},
		{"latest", "Category::latest('", true, "latest", "", "'"},

		// Array arguments
		{"array first element quoted", "Category::with(['", true, "with", "", "'"},
		{"array first element partial", "Category::with(['pro", true, "with", "pro", "'"},
		{"array after comma", "Category::select(['id', '", true, "select", "", "'"},
		{"array after comma partial", "Category::select(['id', 'na", true, "select", "na", "'"},
		{"array no quote", "Category::with([", true, "with", "", ""},
		{"array after comma no quote", "Category::select(['id', ", true, "select", "", ""},
		{"array double quote", `Category::with(["pro`, true, "with", "pro", "\""},
		{"get with array", "Category::get(['", true, "get", "", "'"},

		// Should NOT match
		{"closed paren", "Category::where('name')", false, "", "", ""},
		{"second arg", "Category::where('name', ", false, "", "", ""},
		{"unknown method", "$foo->someCustomMethod('", false, "", "", ""},
		{"variable arg", "Category::where($col", false, "", "", ""},
		{"closure arg", "Category::where(function", false, "", "", ""},
		{"where array not supported", "Category::where(['", false, "", "", ""},
		{"closed array", "Category::with(['products'])", false, "", "", ""},
		{"assoc array", "Category::where(['name' =>", false, "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, partial, quote, ok := extractBuilderArgContext(tt.input)
			if ok != tt.wantOk {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOk)
			}
			if !tt.wantOk {
				return
			}
			if method != tt.wantMeth {
				t.Errorf("method = %q, want %q", method, tt.wantMeth)
			}
			if partial != tt.wantPart {
				t.Errorf("partial = %q, want %q", partial, tt.wantPart)
			}
			if quote != tt.wantQt {
				t.Errorf("quote = %q, want %q", quote, tt.wantQt)
			}
		})
	}
}

func TestBuilderColumnCompletion(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)

	t.Run("Category::where(' suggests column names", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::where('"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 17})
		labels := collectLabels(items)

		for _, col := range []string{"id", "name", "slug", "description", "is_active", "sort_order", "created_at", "updated_at"} {
			if !labels[col] {
				t.Errorf("expected column %q, got labels: %v", col, labels)
			}
		}
		// Should not suggest relation methods
		if labels["products"] {
			t.Error("should not suggest relation method 'products' for column context")
		}
	})

	t.Run("Category::where('na filters to matching columns", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::where('na"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		if !labels["name"] {
			t.Errorf("expected 'name' in filtered results, got: %v", labels)
		}
		if labels["id"] {
			t.Error("'id' should be filtered out by 'na' prefix")
		}
	})

	t.Run("Category::orderBy(' suggests columns", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::orderBy('"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		if !labels["name"] {
			t.Errorf("expected column 'name' for orderBy, got: %v", labels)
		}
		if !labels["created_at"] {
			t.Errorf("expected column 'created_at' for orderBy, got: %v", labels)
		}
	})

	t.Run("Product::where('ca filters to category_id", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Product;\nProduct::where('ca"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 18})
		labels := collectLabels(items)

		if !labels["category_id"] {
			t.Errorf("expected 'category_id' for Product, got: %v", labels)
		}
	})

	t.Run("chained query()->where(' suggests columns", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->where('"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 26})
		labels := collectLabels(items)

		if !labels["name"] {
			t.Errorf("expected column 'name' for chained query, got: %v", labels)
		}
	})

	t.Run("no quote wraps in single quotes", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::where("
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 16})

		for _, item := range items {
			if item.Label == "name" {
				if item.InsertText != "'name'" {
					t.Errorf("expected InsertText=\"'name'\" (with quotes), got %q", item.InsertText)
				}
				return
			}
		}
		t.Error("expected 'name' column in results")
	})

	t.Run("double quote uses double quotes", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::where(\""
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 17})

		for _, item := range items {
			if item.Label == "name" {
				if item.InsertText != "name" {
					t.Errorf("expected InsertText=\"name\" (no wrapping, quote already typed), got %q", item.InsertText)
				}
				return
			}
		}
		t.Error("expected 'name' column in results")
	})

	t.Run("select([' suggests columns in array", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::select(['"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		if !labels["id"] {
			t.Errorf("expected column 'id' in array context, got: %v", labels)
		}
		if !labels["name"] {
			t.Errorf("expected column 'name' in array context, got: %v", labels)
		}
	})

	t.Run("select(['id', ' suggests more columns after comma", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::select(['id', '"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 25})
		labels := collectLabels(items)

		if !labels["name"] {
			t.Errorf("expected column 'name' after comma in array, got: %v", labels)
		}
	})

	t.Run("get([' suggests only DB columns", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::get(['"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 16})
		labels := collectLabels(items)

		// Migration-derived columns (not shadowed by declared properties) should appear
		for _, col := range []string{"id", "description", "is_active", "sort_order", "created_at", "updated_at"} {
			if !labels[col] {
				t.Errorf("expected DB column %q for get(['), got: %v", col, labels)
			}
		}
		// Declared properties (public string $name) are NOT DB-discovered columns
		if labels["name"] || labels["slug"] {
			t.Error("declared PHP properties should not appear in get() — only DB-discovered columns")
		}
		// Relation virtual properties should NOT appear
		if labels["products"] {
			t.Error("should not suggest relation 'products' for get()")
		}
	})
}

func TestBuilderRelationCompletion(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)

	t.Run("Category::with(' suggests relations", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::with('"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 16})
		labels := collectLabels(items)

		if !labels["products"] {
			t.Errorf("expected relation 'products', got: %v", labels)
		}
		// Should not suggest columns
		if labels["name"] {
			t.Error("should not suggest column 'name' for relation context")
		}
	})

	t.Run("Product::whereHas(' suggests relations", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Product;\nProduct::whereHas('"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		if !labels["category"] {
			t.Errorf("expected relation 'category', got: %v", labels)
		}
	})

	t.Run("Category::has(' suggests relations", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::has('"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 15})
		labels := collectLabels(items)

		if !labels["products"] {
			t.Errorf("expected relation 'products', got: %v", labels)
		}
	})

	t.Run("Category::with([' suggests relations in array", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::with(['"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 17})
		labels := collectLabels(items)

		if !labels["products"] {
			t.Errorf("expected relation 'products' in array context, got: %v", labels)
		}
		if labels["name"] {
			t.Error("should not suggest column 'name' for relation array context")
		}
	})

	t.Run("non-Builder method returns nothing", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::someCustomMethod('"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 28})

		// Should fall through to other completion, not column/relation
		for _, item := range items {
			if item.Label == "name" && item.Kind == protocol.CompletionItemKindProperty {
				t.Error("should not suggest column for unknown method")
			}
		}
	})
}

// TestResolveBuilderModelForDiagnostics checks the model the unknown-column
// and unknown-relation diagnostics get for a call, from the line up to the
// call's opening parenthesis.
func TestResolveBuilderModelForDiagnostics(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)
	source := "<?php\nuse App\\Models\\Category;\nCategory::query()->where('name', 1)->with('products');\n"
	file := parser.ParseFile(source, token.Latest)
	for _, tc := range []struct{ prefix, method, want string }{
		{"Category::query()->where(", "where", `App\Models\Category`},
		{"Category::query()->where('name', 1)->with(", "with", `App\Models\Category`},
		{"$unknown->where(", "where", ""},
		{"where(", "where", ""},
		{"::where(", "where", ""},
	} {
		if got := p.ResolveBuilderModel(tc.prefix, tc.method, source, 2, file).FQN; got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.prefix, got, tc.want)
		}
	}
}

func TestBuilderColumnCompletionOnLessCommonReceivers(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)
	category := p.index.Lookup(`App\Models\Category`)
	if category == nil {
		t.Fatal("the fixture indexes Category")
	}
	p.index.SetScopeConventions(laravel.ScopeConventions())
	p.index.IndexFile(category.URI, strings.Replace(p.index.GetFileSource(category.URI), "public string $slug;",
		"public string $slug;\n\n    public function scopeActive($query) { return $query; }", 1))

	for _, body := range []string{
		"(new Category())->where('",
		"$make = fn(): Category => new Category();\n$make()->where('",
		"Category::active()->where('",
	} {
		source := "<?php\nuse App\\Models\\Category;\n" + body
		lines := strings.Split(source, "\n")
		last := len(lines) - 1
		labels := collectLabels(p.GetCompletions("file:///test.php", source, protocol.Position{Line: last, Character: len(lines[last])}))
		if !labels["name"] || !labels["slug"] {
			t.Errorf("%q suggests Category's columns, got %v", body, labels)
		}
	}
}

func TestBuilderColumnCompletionOnABuilderOrRelationHeldElsewhere(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)
	p.index.IndexFile("file:///vendor/Relation.php", "<?php\nnamespace Illuminate\\Database\\Eloquent\\Relations;\nabstract class Relation {\n    public function where($column, $operator = null, $value = null): static { return $this; }\n}\n")
	p.index.IndexFile("file:///vendor/HasMany.php", "<?php\nnamespace Illuminate\\Database\\Eloquent\\Relations;\nclass HasMany extends Relation {}\n")
	category := p.index.Lookup(`App\Models\Category`)
	if category == nil {
		t.Fatal("the fixture indexes Category")
	}
	p.index.IndexFile(category.URI, strings.Replace(p.index.GetFileSource(category.URI), "    public function products(): HasMany",
		"    /** @return HasMany<Product, $this> */\n    public function products(): HasMany", 1))

	for _, tc := range []struct {
		body     string
		wantName string
	}{
		{"$q = Category::query();\n$q->where('", "slug"},
		{"$q = Category::query()->where('slug', 'a');\n$q->orderBy('", "slug"},
		{"/** @param \\Illuminate\\Database\\Eloquent\\Builder<Category> $q */\nfunction scoped($q) {\n    $q->where('", "slug"},
		{"$category = new Category();\n$category->products()->where('", "category_id"},
	} {
		source := "<?php\nuse App\\Models\\Category;\n" + tc.body
		lines := strings.Split(source, "\n")
		last := len(lines) - 1
		labels := collectLabels(p.GetCompletions("file:///test.php", source, protocol.Position{Line: last, Character: len(lines[last])}))
		if !labels[tc.wantName] {
			t.Errorf("%q offers the queried model's columns, got %v", tc.body, labels)
		}
	}

	p.index.IndexFile("file:///app/Helper.php", "<?php\nnamespace App;\nclass Helper {\n    public static function where($column) {}\n}\n")
	for _, source := range []string{
		"<?php\nfunction any(\\Illuminate\\Database\\Eloquent\\Builder $q) {\n    $q->where('",
		"<?php\nuse App\\Helper;\nHelper::where('",
	} {
		lines := strings.Split(source, "\n")
		last := len(lines) - 1
		if items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: last, Character: len(lines[last])}); len(items) != 0 {
			t.Errorf("%q has no model to take columns from, got %v", source, collectLabels(items))
		}
	}
}

func BenchmarkBuilderColumnCompletion(b *testing.B) {
	p, _ := setupEloquentCompletionTest(b)
	for _, source := range []string{
		"<?php\nuse App\\Models\\Category;\nCategory::query()->where('",
		"<?php\nuse App\\Models\\Category;\n$q = Category::query();\n$q->where('",
	} {
		lines := strings.Split(source, "\n")
		pos := protocol.Position{Line: len(lines) - 1, Character: len(lines[len(lines)-1])}
		b.Run(lines[len(lines)-1], func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p.GetCompletions("file:///test.php", source, pos)
			}
		})
	}
}
