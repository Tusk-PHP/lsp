package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const nestedRelationModels = `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Relations\BelongsToMany;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Database\Eloquent\Relations\HasManyThrough;
use Illuminate\Database\Eloquent\Relations\MorphMany;
use Illuminate\Database\Eloquent\Relations\MorphTo;
use Illuminate\Database\Eloquent\Relations\MorphToMany;

class Category extends Model {
    public function products(): HasMany { return $this->hasMany(Product::class); }
    public function tags(): BelongsToMany { return $this->belongsToMany(Tag::class); }
    public function comments(): MorphMany { return $this->morphMany(Comment::class, 'commentable'); }
    public function labels(): MorphToMany { return $this->morphToMany(Tag::class, 'labelable'); }
    public function followers(): MorphToMany { return $this->morphedByMany(Tag::class, 'followable'); }
    public function orders(): HasManyThrough { return $this->hasManyThrough(Order::class, Product::class); }
    public function untyped() { return $this->hasMany(Product::class); }
    public function parent(): BelongsTo { return $this->belongsTo(Category::class); }
}
class Product extends Model {
    public function reviews(): HasMany { return $this->hasMany(Review::class); }
}
class Tag extends Model {
    public function reviews(): HasMany { return $this->hasMany(Review::class); }
}
class Comment extends Model {
    public function reviews(): HasMany { return $this->hasMany(Review::class); }
    public function commentable(): MorphTo { return $this->morphTo(); }
}
class Order extends Model {
    public function reviews(): HasMany { return $this->hasMany(Review::class); }
}
class Review extends Model {}
`

func analyzeNestedRelationPaths(t *testing.T, body string) []string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for _, uri := range []string{"file:///vendor/Model.php", "file:///vendor/Relations.php"} {
		idx.IndexFile(uri, eloquentFiles[uri])
	}
	idx.IndexFile("file:///vendor/Builder.php", `<?php
namespace Illuminate\Database\Eloquent;
/** @template TModel */
class Builder {
    public function with($relations): static { return $this; }
    public function whereHas($relation, $callback = null): static { return $this; }
    public function withSum($relation, $column): static { return $this; }
    public function get() {}
}
`)
	idx.IndexFile("file:///vendor/MorphMany.php", "<?php\nnamespace Illuminate\\Database\\Eloquent\\Relations;\nclass MorphMany extends Relation {}\n")
	dir := t.TempDir()
	path := filepath.Join(dir, "Models.php")
	if err := os.WriteFile(path, []byte(nestedRelationModels), 0o644); err != nil {
		t.Fatal(err)
	}
	idx.IndexFile("file://"+path, nestedRelationModels)
	idx.AddVirtualMember(`App\Models\Product`, &symbols.Symbol{
		Name: "$price", FQN: `App\Models\Product::$price`, Kind: symbols.KindProperty,
		IsVirtual: true, Type: "mixed", DocComment: "(database column) decimal",
	})
	idx.SetCallForwards(laravel.CallForwards())
	models.AnalyzeEloquentModels(idx, dir)
	src := "<?php\nnamespace App\\Http;\n\nuse App\\Models\\Category;\n\nfunction run(): void\n{\n" + body + "}\n"
	idx.IndexFile("file:///app/Http/Run.php", src)
	idx.MarkReady()
	p := NewProvider(idx, "laravel", "/tmp", nil, config.DefaultConfig())
	p.ResolveTypes(nil)
	p.TypeResolver = nil
	p.BuilderMemberChecker = NewIndexMemberChecker(idx, func() bool { return true })
	var got []string
	for _, d := range p.AnalyzeAll("file:///app/Http/Run.php", src) {
		if d.Code == "unknown-relation" || d.Code == "unknown-column" {
			got = append(got, d.Message)
		}
	}
	return got
}

func TestNestedRelationPathsAreCheckedThroughEveryRelation(t *testing.T) {
	for relation, model := range map[string]string{
		"products":        "Product",
		"tags":            "Tag",
		"comments":        "Comment",
		"labels":          "Tag",
		"followers":       "Tag",
		"orders":          "Order",
		"untyped":         "Product",
		"parent.products": "Product",
	} {
		t.Run(relation, func(t *testing.T) {
			if got := analyzeNestedRelationPaths(t, "    Category::with('"+relation+".reviews')->get();\n"); len(got) != 0 {
				t.Errorf("%s.reviews is a valid path, got %v", relation, got)
			}
			want := "Unknown relation 'missing' on model '" + model + "'"
			if got := analyzeNestedRelationPaths(t, "    Category::with('"+relation+".missing')->get();\n"); strings.Join(got, "\n") != want {
				t.Errorf("%s.missing: got %v, want %s", relation, got, want)
			}
		})
	}
}

func TestNestedRelationPathsStayQuietPastAModelTheyCantTell(t *testing.T) {
	if got := analyzeNestedRelationPaths(t, "    Category::with('comments.commentable.anything')->whereHas('products.reviews.anything')->get();\n"); strings.Join(got, "\n") != "Unknown relation 'anything' on model 'Review'" {
		t.Errorf("a morphTo's model is unknown, so nothing past it is checked; got %v", got)
	}
}

func TestAggregateColumnsAreCheckedOnAToManyRelatedModel(t *testing.T) {
	got := analyzeNestedRelationPaths(t, "    Category::withSum('products', 'price')->withSum('products', 'missing')->get();\n")
	if want := "Unknown column 'missing' on related model 'Product'"; strings.Join(got, "\n") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}
