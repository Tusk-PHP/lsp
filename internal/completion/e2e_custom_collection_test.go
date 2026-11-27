package completion

import (
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestModelCollectionsUseTheModelsOwnCollectionClass(t *testing.T) {
	requireLaravelVendor(t)
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	indexLaravelVendor(idx, filepath.Join("..", "..", "testdata", "laravel"))
	dir := t.TempDir()
	index := func(name, src string) {
		writeProjectFile(t, dir, name, src)
		idx.IndexFile("file://"+filepath.Join(dir, name), src)
	}
	index("Category.php", `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsToMany;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Category extends Model
{
    public function products(): HasMany { return $this->hasMany(Product::class); }
    public function tags(): BelongsToMany { return $this->belongsToMany(Tag::class); }
    public function labels(): HasMany { return $this->hasMany(Label::class); }
    public function plain(): HasMany { return $this->hasMany(Plain::class); }
}
`)
	index("Product.php", `<?php
namespace App\Models;

use App\Collections\ProductCollection;
use Illuminate\Database\Eloquent\Attributes\CollectedBy;
use Illuminate\Database\Eloquent\Model;

#[CollectedBy(ProductCollection::class)]
class Product extends Model
{
    public function productSku(): string { return ''; }
}
`)
	index("Tag.php", `<?php
namespace App\Models;

use App\Collections\TagCollection;
use Illuminate\Database\Eloquent\Collection;
use Illuminate\Database\Eloquent\Model;

class Tag extends Model
{
    public function newCollection(array $models = []): Collection
    {
        return new TagCollection($models);
    }
    public function tagColor(): string { return ''; }
}
`)
	index("Label.php", `<?php
namespace App\Models;

use App\Collections\LabelCollection;
use Illuminate\Database\Eloquent\Model;

class Label extends Model
{
    public function newCollection(array $models = [])
    {
        return new LabelCollection($models);
    }
    public function labelText(): string { return ''; }
}
`)
	index("Plain.php", "<?php\nnamespace App\\Models;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass Plain extends Model { public function plainName(): string { return ''; } }\n")
	for class, method := range map[string]string{"ProductCollection": "totalSku", "TagCollection": "colors", "LabelCollection": "texts"} {
		index(class+".php", "<?php\nnamespace App\\Collections;\nuse Illuminate\\Database\\Eloquent\\Collection;\nclass "+class+" extends Collection { public function "+method+"(): array { return []; } }\n")
	}
	idx.SetCallForwards(laravel.CallForwards())
	idx.SetScopeConventions(laravel.ScopeConventions())
	idx.SetCollectionFactories(laravel.CollectionFactories())
	models.AnalyzeEloquentModels(idx, dir)
	p := NewProvider(idx, nil, "laravel")

	in := func(body string) string {
		return "<?php\nuse App\\Models\\Category;\nuse App\\Models\\Product;\nuse App\\Models\\Tag;\nuse App\\Models\\Label;\nfunction f(Category $category) {\n    " + body + "\n}\n"
	}
	for _, c := range []struct {
		name, body, want string
	}{
		{"#[CollectedBy] relation", "$category->products->|", "totalSku"},
		{"#[CollectedBy] get()", "Product::query()->where('a', 1)->get()->|", "totalSku"},
		{"#[CollectedBy] all()", "Product::all()->|", "totalSku"},
		{"typed newCollection relation", "$category->tags->|", "colors"},
		{"typed newCollection get()", "Tag::query()->get()->|", "colors"},
		{"untyped newCollection relation", "$category->labels->|", "texts"},
		{"item of a custom collection", "foreach ($category->products as $product) {\n        $product->|\n    }", "productSku"},
		{"first() of a custom collection", "$category->labels->first()->|", "labelText"},
		{"plain model keeps Eloquent's collection", "$category->plain->first()->|", "plainName"},
		{"relation method stays the relation", "$category->products()->|", "saveMany"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if labels := completeAtMarker(p, in(c.body)); !labels[c.want] {
				t.Errorf("%s should offer %s(), got %v", c.body, c.want, mapKeys(labels, 20))
			}
		})
	}
}
