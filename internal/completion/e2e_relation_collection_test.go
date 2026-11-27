package completion

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type toManyRelation struct {
	property      string
	relatedMethod string
	builderMethod string
}

var laravelToManyRelations = []toManyRelation{
	{"products", "productSku", "saveMany"},
	{"untypedProducts", "productSku", "saveMany"},
	{"tags", "tagColor", "attach"},
	{"comments", "commentBody", "saveMany"},
	{"labels", "labelText", "attach"},
	{"followers", "followerHandle", "attach"},
	{"orders", "orderTotal", "withTrashedParents"},
}

func setupToManyRelations(t *testing.T) *Provider {
	t.Helper()
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
use Illuminate\Database\Eloquent\Relations\HasManyThrough;
use Illuminate\Database\Eloquent\Relations\MorphMany;
use Illuminate\Database\Eloquent\Relations\MorphToMany;

class Category extends Model
{
    public function products(): HasMany { return $this->hasMany(Product::class); }
    public function untypedProducts() { return $this->hasMany(Product::class); }
    public function tags(): BelongsToMany { return $this->belongsToMany(Tag::class); }
    public function comments(): MorphMany { return $this->morphMany(Comment::class, 'commentable'); }
    public function labels(): MorphToMany { return $this->morphToMany(Label::class, 'labelable'); }
    public function followers(): MorphToMany { return $this->morphedByMany(Follower::class, 'followable'); }
    public function orders(): HasManyThrough { return $this->hasManyThrough(Order::class, Product::class); }
}
`)
	for class, method := range map[string]string{
		"Product":  "productSku",
		"Tag":      "tagColor",
		"Comment":  "commentBody",
		"Label":    "labelText",
		"Follower": "followerHandle",
		"Order":    "orderTotal",
	} {
		index(class+".php", "<?php\nnamespace App\\Models;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass "+class+" extends Model { public function "+method+"(): string { return ''; } }\n")
	}
	idx.SetCallForwards(laravel.CallForwards())
	idx.SetScopeConventions(laravel.ScopeConventions())
	models.AnalyzeEloquentModels(idx, dir)
	return NewProvider(idx, nil, "laravel")
}

func completeAtMarker(p *Provider, src string) map[string]bool {
	before, after, _ := strings.Cut(src, "|")
	line := strings.Count(before, "\n")
	col := len(before) - strings.LastIndexByte(before, '\n') - 1
	return collectLabels(p.GetCompletions("file:///test.php", before+after, protocol.Position{Line: line, Character: col}))
}

func TestToManyRelationItemsAreTheRelatedModel(t *testing.T) {
	p := setupToManyRelations(t)
	for _, rel := range laravelToManyRelations {
		t.Run(rel.property, func(t *testing.T) {
			loop := "<?php\nuse App\\Models\\Category;\nfunction f(Category $category) {\n    foreach ($category->" + rel.property + " as $item) {\n        $item->|\n    }\n}\n"
			if labels := completeAtMarker(p, loop); !labels[rel.relatedMethod] {
				t.Errorf("loop item of $category->%s should be the related model with %s(), got %v", rel.property, rel.relatedMethod, mapKeys(labels, 20))
			}
			first := "<?php\nuse App\\Models\\Category;\nfunction f(Category $category) {\n    $category->" + rel.property + "->first()->|\n}\n"
			if labels := completeAtMarker(p, first); !labels[rel.relatedMethod] {
				t.Errorf("$category->%s->first() should be the related model with %s(), got %v", rel.property, rel.relatedMethod, mapKeys(labels, 20))
			}
			call := "<?php\nuse App\\Models\\Category;\nfunction f(Category $category) {\n    $category->" + rel.property + "()->|\n}\n"
			if labels := completeAtMarker(p, call); !labels[rel.builderMethod] {
				t.Errorf("$category->%s() should stay the relation with %s(), got %v", rel.property, rel.builderMethod, mapKeys(labels, 20))
			}
		})
	}
}

func TestUntypedRelationIsOfferedAsAnEagerLoad(t *testing.T) {
	p := setupToManyRelations(t)
	labels := completeAtMarker(p, "<?php\nuse App\\Models\\Category;\nCategory::with('|');\n")
	for _, rel := range laravelToManyRelations {
		if !labels[rel.property] {
			t.Errorf("with() should offer the %s relation, got %v", rel.property, mapKeys(labels, 20))
		}
	}
}

func TestToManyRelationDocblockPropertyWins(t *testing.T) {
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
use Illuminate\Database\Eloquent\Relations\HasMany;

/**
 * @property-read \Illuminate\Database\Eloquent\Collection<int, Featured> $products
 */
class Category extends Model
{
    public function products(): HasMany { return $this->hasMany(Product::class); }
}
`)
	index("Product.php", "<?php\nnamespace App\\Models;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass Product extends Model { public function productSku(): string { return ''; } }\n")
	index("Featured.php", "<?php\nnamespace App\\Models;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass Featured extends Model { public function featuredRank(): int { return 0; } }\n")
	idx.SetCallForwards(laravel.CallForwards())
	models.AnalyzeEloquentModels(idx, dir)
	p := NewProvider(idx, nil, "laravel")

	labels := completeAtMarker(p, "<?php\nuse App\\Models\\Category;\nfunction f(Category $category) {\n    foreach ($category->products as $item) {\n        $item->|\n    }\n}\n")
	if !labels["featuredRank"] || labels["productSku"] {
		t.Errorf("the class docblock's @property-read should type the items, got %v", mapKeys(labels, 20))
	}
}

const doctrineCollectionsStub = `<?php
namespace Doctrine\Common\Collections;

/**
 * @template TKey of array-key
 * @template-covariant T
 * @extends \IteratorAggregate<TKey, T>
 */
interface ReadableCollection extends \Countable, \IteratorAggregate
{
    /** @return T|false */
    public function first();
}

/**
 * @template TKey of array-key
 * @template T
 * @extends ReadableCollection<TKey, T>
 * @extends \ArrayAccess<TKey, T>
 */
interface Collection extends ReadableCollection, \ArrayAccess
{
    /** @param T $element */
    public function add(mixed $element): void;
}
`

func TestDoctrineToManyItemsAreTheTargetEntity(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///vendor/doctrine/collections/Collection.php", doctrineCollectionsStub)
	dir := t.TempDir()
	index := func(name, src string) {
		writeProjectFile(t, dir, name, src)
		idx.IndexFile("file://"+filepath.Join(dir, name), src)
	}
	index("Post.php", `<?php
namespace App\Entity;

use Doctrine\Common\Collections\Collection;
use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity]
class Post
{
    #[ORM\OneToMany(targetEntity: Comment::class, mappedBy: 'post')]
    private Collection $comments;

    #[ORM\ManyToMany(targetEntity: Tag::class)]
    private Collection $tags;

    #[ORM\ManyToMany(targetEntity: Tag::class)]
    private $untypedTags;
}
`)
	index("Comment.php", "<?php\nnamespace App\\Entity;\nclass Comment { public function commentBody(): string { return ''; } }\n")
	index("Tag.php", "<?php\nnamespace App\\Entity;\nclass Tag { public function tagColor(): string { return ''; } }\n")
	models.AnalyzeDoctrineEntities(idx, dir)
	p := NewProvider(idx, nil, "symfony")

	for prop, method := range map[string]string{"comments": "commentBody", "tags": "tagColor", "untypedTags": "tagColor"} {
		t.Run(prop, func(t *testing.T) {
			loop := "<?php\nnamespace App\\Entity;\nfunction f(Post $post) {\n    foreach ($post->" + prop + " as $item) {\n        $item->|\n    }\n}\n"
			if labels := completeAtMarker(p, loop); !labels[method] {
				t.Errorf("loop item of $post->%s should be the target entity with %s(), got %v", prop, method, mapKeys(labels, 20))
			}
			first := "<?php\nnamespace App\\Entity;\nfunction f(Post $post) {\n    $post->" + prop + "->first()->|\n}\n"
			if labels := completeAtMarker(p, first); !labels[method] {
				t.Errorf("$post->%s->first() should be the target entity with %s(), got %v", prop, method, mapKeys(labels, 20))
			}
		})
	}
}
