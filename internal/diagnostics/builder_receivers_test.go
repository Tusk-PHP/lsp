package diagnostics

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var eloquentFiles = map[string]string{
	"file:///vendor/Model.php": `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {
    /** @return Builder<static> */
    public static function query() {}
}
`,
	"file:///vendor/Builder.php": `<?php
namespace Illuminate\Database\Eloquent;
/** @template TModel */
class Builder {
    public function where($column, $operator = null, $value = null): static { return $this; }
    public function with($relations): static { return $this; }
    public function join($table, $first, $operator = null, $second = null): static { return $this; }
    public function leftJoin($table, $first, $operator = null, $second = null): static { return $this; }
    public function crossJoin($table): static { return $this; }
    public function joinSub($query, $as, $first): static { return $this; }
}
`,
	"file:///vendor/Relations.php": `<?php
namespace Illuminate\Database\Eloquent\Relations;
/**
 * @template TRelatedModel
 * @template TDeclaringModel
 */
abstract class Relation {
    public function where($column, $operator = null, $value = null): static { return $this; }
    public function with($relations): static { return $this; }
}
class HasMany extends Relation {}
class BelongsTo extends Relation {}
class BelongsToMany extends Relation {}
class MorphToMany extends BelongsToMany {}
class HasManyThrough extends Relation {}
class HasOneThrough extends Relation {}
class MorphTo extends BelongsTo {}
`,
	"file:///app/Models.php": `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsToMany;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Database\Eloquent\Relations\HasManyThrough;
use Illuminate\Database\Eloquent\Relations\HasOneThrough;
use Illuminate\Database\Eloquent\Relations\MorphTo;
use Illuminate\Database\Eloquent\Relations\MorphToMany;

class Post extends Model {
    /** @return HasMany<Comment, $this> */
    public function comments(): HasMany { return $this->hasMany(Comment::class); }
}
class Video extends Model {
    /** @return HasMany<Comment, $this> */
    public function clips(): HasMany { return $this->hasMany(Comment::class); }
}
class Comment extends Model {
    /** @return MorphTo<Post|Video, $this> */
    public function commentable(): MorphTo { return $this->morphTo(); }
}
class Product extends Model {}
class Tag extends Model {
    /** @return BelongsToMany<Product, $this> */
    public function products(): BelongsToMany { return $this->belongsToMany(Product::class); }
    /** @return MorphToMany<Post, $this> */
    public function posts(): MorphToMany { return $this->morphedByMany(Post::class, 'taggable'); }
    /** @return HasMany<Product, $this> */
    public function featured(): HasMany { return $this->hasMany(Product::class); }
}
class Country extends Model {
    /** @return HasManyThrough<Post, User, $this> */
    public function posts(): HasManyThrough { return $this->hasManyThrough(Post::class, User::class); }
    /** @return HasOneThrough<Post, User, $this> */
    public function latestPost(): HasOneThrough { return $this->hasOneThrough(Post::class, User::class); }
}
class User extends Model {}
`,
}

var eloquentColumns = map[string][]string{
	`App\Models\Post`:    {"id", "title", "user_id"},
	`App\Models\Video`:   {"id", "duration"},
	`App\Models\Comment`: {"id", "body"},
	`App\Models\Product`: {"id", "title"},
	`App\Models\Tag`:     {"id", "name"},
	`App\Models\Country`: {"id", "name"},
	`App\Models\User`:    {"id", "email"},
}

func analyzeBuilderArgs(t *testing.T, body string) []string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for uri, src := range eloquentFiles {
		idx.IndexFile(uri, src)
	}
	for model, columns := range eloquentColumns {
		for _, column := range columns {
			idx.AddVirtualMember(model, &symbols.Symbol{
				Name: "$" + column, FQN: model + "::$" + column, Kind: symbols.KindProperty,
				IsVirtual: true, Type: "mixed", DocComment: "(database column) varchar",
			})
		}
	}
	src := "<?php\nnamespace App\\Http;\n\nuse App\\Models\\Comment;\nuse App\\Models\\Country;\nuse App\\Models\\Post;\nuse App\\Models\\Tag;\n\nfunction run(Tag $tag, Country $country, Comment $comment, string $email): void\n{\n" + body + "}\n"
	idx.IndexFile("file:///app/Http/Run.php", src)
	idx.MarkReady()
	p := NewProvider(idx, "laravel", "/tmp", nil, config.DefaultConfig())
	p.BuilderModelResolver = completion.NewProvider(idx, nil, "laravel").ResolveBuilderModel
	p.BuilderMemberChecker = NewIndexMemberChecker(idx, func() bool { return true })
	var got []string
	for _, d := range p.AnalyzeAll("file:///app/Http/Run.php", src) {
		if d.Code == "unknown-column" || d.Code == "unknown-relation" {
			got = append(got, d.Message)
		}
	}
	return got
}

func assertBuilderArgs(t *testing.T, body string, want ...string) {
	t.Helper()
	if got := analyzeBuilderArgs(t, body); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestBuilderArgsOnARelationCheckTheRelatedModel(t *testing.T) {
	assertBuilderArgs(t, `    $tag->featured()->where('missing', 1);
    $tag->featured()->with('missingRelation');
`,
		"Unknown column 'missing' on model 'Product'",
		"Unknown relation 'missingRelation' on model 'Product'",
	)
}

func TestBuilderArgsOnARelationThatJoinsAnotherTableLeaveColumnsAlone(t *testing.T) {
	assertBuilderArgs(t, `    $tag->products()->where('weight', 1);
    $tag->posts()->where('taggable_type', Post::class);
    $country->posts()->where('country_id', 1);
    $country->latestPost()->where('country_id', 1);
    $tag->products()->with('missingRelation');
`,
		"Unknown relation 'missingRelation' on model 'Product'",
	)
}

func TestBuilderArgsOnAUnionOfModelsAreNotChecked(t *testing.T) {
	assertBuilderArgs(t, `    $comment->commentable()->where('duration', 1);
    $comment->commentable()->with('clips');
`)
}
