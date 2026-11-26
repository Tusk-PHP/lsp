package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// TestDefinitionOfAPropertyMadeFromAModelMethod covers the properties model
// analysis adds for a relation and for both kinds of accessor. A plain access
// prefers a property over a method of the same name, so these properties must
// point at the method that declares them, not at the top of the model's file.
func TestDefinitionOfAPropertyMadeFromAModelMethod(t *testing.T) {
	sources := map[string]string{
		"Model.php": `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {}
`,
		"HasMany.php": `<?php
namespace Illuminate\Database\Eloquent\Relations;
class HasMany {}
`,
		"Attribute.php": `<?php
namespace Illuminate\Database\Eloquent\Casts;
class Attribute { public static function make(...$a): static { return new static; } }
`,
		"Product.php": `<?php
namespace App;
use Illuminate\Database\Eloquent\Model;
class Product extends Model {}
`,
		"Category.php": `<?php
namespace App;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Database\Eloquent\Casts\Attribute;
class Category extends Model
{
    public function products(): HasMany
    {
        return $this->hasMany(Product::class);
    }

    public function getFullNameAttribute(): string
    {
        return '';
    }

    protected function slug(): Attribute
    {
        return Attribute::make(get: fn () => '');
    }
}
`,
	}
	use := `<?php
namespace App;
class Shop
{
    public function show(Category $category)
    {
        $category->products;
        $category->full_name;
        $category->slug;
    }
}
`
	// Model analysis reads the model's file from disk.
	dir := t.TempDir()
	uriOf := func(name string) string { return symbols.PathToURI(filepath.Join(dir, name)) }
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for name, src := range sources {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		idx.IndexFileWithSource(uriOf(name), src, symbols.SourceProject)
	}
	idx.IndexFileWithSource(uriOf("Shop.php"), use, symbols.SourceProject)
	models.AnalyzeEloquentModels(idx, "")
	if idx.Lookup(`App\Category::$products`) == nil {
		t.Fatal("expected model analysis to add $products")
	}
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))

	for needle, wantLine := range map[string]int{
		"products;":  7,  // public function products()
		"full_name;": 12, // public function getFullNameAttribute()
		"slug;":      17, // protected function slug()
	} {
		got := a.FindDefinition(uriOf("Shop.php"), use, positionOf(t, use, needle))
		if got == nil || got.URI != uriOf("Category.php") || got.Range.Start.Line != wantLine {
			t.Errorf("definition of $category->%s: got %+v, want Category.php line %d", needle, got, wantLine)
		}
	}
}
