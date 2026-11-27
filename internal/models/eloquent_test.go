package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func setupEloquentIndex(t *testing.T, modelSource string) *symbols.Index {
	t.Helper()
	idx := symbols.NewIndex()

	// Index a minimal Eloquent Model base class
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Model.php", `<?php
namespace Illuminate\Database\Eloquent;

abstract class Model {
    public function save(): bool { return true; }
    public function delete(): bool { return true; }
}
`)

	// Index the Attribute cast class
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Casts/Attribute.php", `<?php
namespace Illuminate\Database\Eloquent\Casts;

class Attribute {
    public static function make(): static { return new static(); }
}
`)

	// Index relation stubs
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Relations/HasMany.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;

class HasMany {}
`)
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Relations/BelongsTo.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;

class BelongsTo {}
`)
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Relations/HasOne.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;

class HasOne {}
`)

	// Index the Collection class
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Collection.php", `<?php
namespace Illuminate\Database\Eloquent;

class Collection {}
`)

	// Write the model source to a temp file so the analyzer can read it
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "User.php")
	if err := os.WriteFile(modelPath, []byte(modelSource), 0644); err != nil {
		t.Fatal(err)
	}

	idx.IndexFile("file://"+modelPath, modelSource)

	return idx
}

func TestEloquentRelationDiscovery(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Relations\HasOne;

class User extends Model {
    public string $email;

    public function posts(): HasMany
    {
        return $this->hasMany(Post::class);
    }

    public function team(): BelongsTo
    {
        return $this->belongsTo(Team::class);
    }

    public function profile(): HasOne
    {
        return $this->hasOne(Profile::class);
    }
}
`
	idx := setupEloquentIndex(t, source)

	// Index related models so resolve works
	idx.IndexFile("file:///Post.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Post extends Model {}
`)
	idx.IndexFile("file:///Team.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Team extends Model {}
`)
	idx.IndexFile("file:///Profile.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Profile extends Model {}
`)

	AnalyzeEloquentModels(idx, "")

	t.Run("HasMany creates Collection virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$posts")
		if sym == nil {
			t.Fatal("expected virtual property '$posts'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual")
		}
		if sym.Type != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Post>" {
			t.Errorf("expected Collection<int, Post> type, got %q", sym.Type)
		}
	})

	t.Run("BelongsTo creates nullable virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$team")
		if sym == nil {
			t.Fatal("expected virtual property '$team'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual")
		}
		if sym.Type != "?App\\Models\\Team" {
			t.Errorf("expected nullable Team type, got %q", sym.Type)
		}
	})

	t.Run("HasOne creates nullable virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$profile")
		if sym == nil {
			t.Fatal("expected virtual property '$profile'")
		}
		if sym.Type != "?App\\Models\\Profile" {
			t.Errorf("expected nullable Profile type, got %q", sym.Type)
		}
	})

	t.Run("real property not affected", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$email")
		if sym == nil {
			t.Fatal("expected real property '$email'")
		}
		if sym.IsVirtual {
			t.Error("real property should not be virtual")
		}
	})
}

func TestEloquentLegacyAccessor(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class User extends Model {
    public function getFullNameAttribute(): string
    {
        return $this->first_name . ' ' . $this->last_name;
    }

    public function getAgeAttribute(): int
    {
        return 25;
    }
}
`
	idx := setupEloquentIndex(t, source)
	AnalyzeEloquentModels(idx, "")

	t.Run("legacy accessor creates virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$full_name")
		if sym == nil {
			t.Fatal("expected virtual property '$full_name'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string', got %q", sym.Type)
		}
	})

	t.Run("another legacy accessor", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$age")
		if sym == nil {
			t.Fatal("expected virtual property '$age'")
		}
		if sym.Type != "int" {
			t.Errorf("expected type 'int', got %q", sym.Type)
		}
	})
}

func TestEloquentModernAccessor(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Casts\Attribute;

class User extends Model {
    /**
     * @return Attribute
     */
    protected function fullName(): Attribute
    {
        return Attribute::make(
            get: fn () => $this->first_name . ' ' . $this->last_name,
        );
    }
}
`
	idx := setupEloquentIndex(t, source)
	AnalyzeEloquentModels(idx, "")

	t.Run("modern accessor creates virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$fullName")
		if sym == nil {
			t.Fatal("expected virtual property '$fullName'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual")
		}
	})
}

func TestEloquentRelationWithoutReturnType(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class User extends Model {
    public function comments()
    {
        return $this->hasMany(Comment::class);
    }
}
`
	idx := setupEloquentIndex(t, source)
	idx.IndexFile("file:///Comment.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Comment extends Model {}
`)
	AnalyzeEloquentModels(idx, "")

	t.Run("relation detected from body without return type", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$comments")
		if sym == nil {
			t.Fatal("expected virtual property '$comments'")
		}
		if sym.Type != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Comment>" {
			t.Errorf("expected Collection<int, Comment> type, got %q", sym.Type)
		}
	})
}

func TestSnakeCase(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"FullName", "full_name"},
		{"Name", "name"},
		{"ID", "i_d"},
		{"CreatedAt", "created_at"},
	}
	for _, tt := range tests {
		if got := snakeCase(tt.input); got != tt.expected {
			t.Errorf("snakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestEloquentModelReadFromTheSyntaxTree(t *testing.T) {
	src := `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Casts\Attribute;
use Illuminate\Database\Eloquent\Relations\HasMany;
class Author extends Model
{
    public function posts(): HasMany { return $this->hasMany(Post::class); }
    public function team()
    {
        return $this
            ->belongsTo(
                Team::class,
            );
    }
    public function other() { return $other->hasMany(Post::class); }
    public function comments() { $relation = $this->hasMany(Comment::class); $relation->latest(); return $relation; }
    public function dynamic(): HasMany { return $this->hasMany($class); }
    public function abstractRelation(): HasMany;
    /** @return string */
    public function getFullNameAttribute() { return ''; }
    public function getattribute() {}
    public function get_Attribute() {}
    public function getFoo_BarAttribute() {}
    protected function title(): Attribute { return Attribute::make(); }
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "Author.php")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.IndexFile("file://"+path, src)
	model := idx.Lookup(`App\Models\Author`)
	analyzeModelSource(idx, model, newModelSource(src, phpparser.OnlyGrammar(token.Latest)))
	for prop, want := range map[string]string{
		"$posts":     `Illuminate\Database\Eloquent\Collection<int, App\Models\Post>`,
		"$comments":  `Illuminate\Database\Eloquent\Collection<int, App\Models\Comment>`,
		"$team":      `?App\Models\Team`,
		"$dynamic":   "mixed",
		"$full_name": "string",
		"$title":     "mixed",
	} {
		if sym := idx.Lookup(`App\Models\Author::` + prop); sym == nil || sym.Type != want {
			t.Errorf("%s: %+v, want %s", prop, sym, want)
		}
	}
	for _, prop := range []string{"$other", "$", "$_", "$foo_Bar"} {
		if idx.Lookup(`App\Models\Author::`+prop) != nil {
			t.Errorf("%s should not be a virtual property", prop)
		}
	}
	var rels []string
	for _, r := range extractEloquentRelations(model, phpparser.OnlyGrammar(token.Latest)) {
		rels = append(rels, r.MethodName+" "+r.RelationType+" "+r.TargetModel)
	}
	if want := []string{`posts HasMany App\Models\Post`, `team BelongsTo App\Models\Team`, `comments HasMany App\Models\Comment`}; strings.Join(rels, "\n") != strings.Join(want, "\n") {
		t.Errorf("relations: %q", rels)
	}
	analyzeModelSource(idx, &symbols.Symbol{FQN: `App\Models\Missing`}, newModelSource(src, phpparser.OnlyGrammar(token.Latest)))
	if got := extractEloquentRelations(&symbols.Symbol{FQN: `App\Models\Author`, URI: "file://" + filepath.Join(dir, "gone.php")}, phpparser.OnlyGrammar(token.Latest)); got != nil {
		t.Errorf("a missing file: %v", got)
	}
	analyzeModel(idx, &symbols.Symbol{FQN: `App\Models\Author`, URI: "file://" + filepath.Join(dir, "gone.php")})
}

func TestEloquentToManyRelationsCarryTheRelatedModel(t *testing.T) {
	src := `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Relations\BelongsToMany;
use Illuminate\Database\Eloquent\Relations\HasManyThrough;
use Illuminate\Database\Eloquent\Relations\MorphMany;
use Illuminate\Database\Eloquent\Relations\MorphToMany;
/**
 * @property-read \Illuminate\Database\Eloquent\Collection<int, Featured> $featured
 */
class Category extends Model
{
    public function tags(): BelongsToMany { return $this->belongsToMany(Tag::class); }
    public function comments(): MorphMany { return $this->morphMany(Comment::class, 'commentable'); }
    public function labels(): MorphToMany { return $this->morphToMany(Label::class, 'labelable'); }
    public function followers() { return $this->morphedByMany(Follower::class, 'followable'); }
    public function orders(): HasManyThrough { return $this->hasManyThrough(Order::class, Product::class); }
    public function featured(): BelongsToMany { return $this->belongsToMany(Product::class); }
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "Category.php")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.IndexFile("file://"+path, src)
	analyzeModelSource(idx, idx.Lookup(`App\Models\Category`), newModelSource(src, phpparser.OnlyGrammar(token.Latest)))
	for prop, want := range map[string]string{
		"$tags":      `Illuminate\Database\Eloquent\Collection<int, App\Models\Tag>`,
		"$comments":  `Illuminate\Database\Eloquent\Collection<int, App\Models\Comment>`,
		"$labels":    `Illuminate\Database\Eloquent\Collection<int, App\Models\Label>`,
		"$followers": `Illuminate\Database\Eloquent\Collection<int, App\Models\Follower>`,
		"$orders":    `Illuminate\Database\Eloquent\Collection<int, App\Models\Order>`,
		"$featured":  `Illuminate\Database\Eloquent\Collection<int, Featured>`,
	} {
		if sym := idx.Lookup(`App\Models\Category::` + prop); sym == nil || sym.Type != want {
			t.Errorf("%s: %+v, want type %s", prop, sym, want)
		}
	}
}

func TestEloquentUntypedRelationMethodsReturnTheRelation(t *testing.T) {
	src := `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Relations\HasMany;
class Category extends Model
{
    public function products() { return $this->hasMany(Product::class); }
    public function followers() { return $this->morphedByMany(Follower::class, 'followable'); }
    public function orders() { return $this->hasManyThrough(Order::class, Product::class); }
    public function owner() { return $this->morphTo(); }
    public function typed(): HasMany { return $this->hasMany(Product::class); }
    /** @return HasMany<Product, $this> */
    public function documented() { return $this->hasMany(Product::class); }
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "Category.php")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.IndexFile("file://"+path, src)
	analyzeModelSource(idx, idx.Lookup(`App\Models\Category`), newModelSource(src, phpparser.OnlyGrammar(token.Latest)))
	for method, want := range map[string]string{
		"products":   `Illuminate\Database\Eloquent\Relations\HasMany<App\Models\Product, App\Models\Category>`,
		"followers":  `Illuminate\Database\Eloquent\Relations\MorphToMany<App\Models\Follower, App\Models\Category>`,
		"orders":     `Illuminate\Database\Eloquent\Relations\HasManyThrough<App\Models\Order, App\Models\Product, App\Models\Category>`,
		"owner":      `Illuminate\Database\Eloquent\Relations\MorphTo`,
		"typed":      `Illuminate\Database\Eloquent\Relations\HasMany`,
		"documented": "",
	} {
		if sym := idx.Lookup(`App\Models\Category::` + method); sym == nil || sym.ReturnType != want {
			t.Errorf("%s(): %+v, want return type %q", method, sym, want)
		}
	}
}

func TestEloquentModelCollectionClass(t *testing.T) {
	src := `<?php
namespace App\Models;
use App\Collections\ProductCollection;
use App\Collections\TagCollection;
use Illuminate\Database\Eloquent\Attributes\CollectedBy;
use Illuminate\Database\Eloquent\Collection;

#[CollectedBy(ProductCollection::class)]
class Product extends Model {}

class Tag extends Model
{
    public function newCollection(array $models = []): Collection { return new TagCollection($models); }
}

class Label extends Model
{
    public function newCollection(array $models = []) { return new \App\Collections\LabelCollection($models); }
}

#[CollectedBy(ProductCollection::class)]
class Overridden extends Model
{
    public function newCollection(array $models = []) { return new static($models); }
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "Models.php")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.IndexFile("file://"+path, src)
	idx.IndexFile("file:///Collection.php", "<?php\nnamespace Illuminate\\Database\\Eloquent;\nclass Collection {}\n")
	idx.IndexFile("file:///Collections.php", "<?php\nnamespace App\\Collections;\nuse Illuminate\\Database\\Eloquent\\Collection;\nclass ProductCollection extends Collection {}\nclass TagCollection extends Collection {}\nclass LabelCollection extends Collection {}\n")
	source := newModelSource(src, phpparser.OnlyGrammar(token.Latest))
	for _, model := range []string{"Product", "Tag", "Label", "Overridden"} {
		analyzeModelSource(idx, idx.Lookup(`App\Models\`+model), source)
	}
	for model, want := range map[string]string{
		"Product":    `App\Collections\ProductCollection`,
		"Tag":        `App\Collections\TagCollection`,
		"Label":      `App\Collections\LabelCollection`,
		"Overridden": "",
	} {
		if sym := idx.Lookup(`App\Models\` + model + `::newCollection`); sym == nil || sym.ReturnType != want {
			t.Errorf("%s::newCollection(): %+v, want return type %q", model, sym, want)
		}
	}
}
