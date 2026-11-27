package hover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverToManyRelationPropertyNamesTheRelatedModel(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", "<?php\nnamespace Illuminate\\Database\\Eloquent;\nabstract class Model {}\n")
	idx.IndexFile("file:///vendor/HasMany.php", "<?php\nnamespace Illuminate\\Database\\Eloquent\\Relations;\nclass HasMany {}\n")
	dir := t.TempDir()
	category := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Category extends Model
{
    public function products(): HasMany { return $this->hasMany(Product::class); }
}
`
	path := filepath.Join(dir, "Category.php")
	if err := os.WriteFile(path, []byte(category), 0o644); err != nil {
		t.Fatal(err)
	}
	idx.IndexFile("file://"+path, category)
	idx.IndexFile("file:///Product.php", "<?php\nnamespace App\\Models;\nclass Product extends \\Illuminate\\Database\\Eloquent\\Model {}\n")
	models.AnalyzeEloquentModels(idx, dir)

	source := `<?php
use App\Models\Category;
function f(Category $category) {
    return $category->products;
}
`
	idx.IndexFile("file:///hover.php", source)
	pos := hoverLineContaining(t, source, "products;")
	hover := NewProvider(idx, nil, "laravel").GetHover("file:///hover.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $category->products")
	}
	if !strings.Contains(hover.Contents.Value, "public Collection<int, Product> $products") {
		t.Errorf("hover should show the related model in the collection type, got:\n%s", hover.Contents.Value)
	}
}
