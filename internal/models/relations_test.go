package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// setupRelationsIndex builds an index with:
//   - the Illuminate\Database\Eloquent\Model base class (via a file URI so
//     URIToPath + os.ReadFile work for the model files being analyzed)
//   - an App\User model with hasMany(Post::class) and hasOne(Profile::class)
//   - App\Post and App\Profile models (needed for FQN resolution)
func setupRelationsIndex(t *testing.T) (*symbols.Index, string) {
	t.Helper()
	idx := symbols.NewIndex()
	tmpDir := t.TempDir()

	// Helper: write a PHP file under tmpDir and index it.
	writeAndIndex := func(name, content string) {
		t.Helper()
		p := filepath.Join(tmpDir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		idx.IndexFile("file://"+p, content)
	}

	// Base Eloquent Model (needs a real file so analyzeModel can os.ReadFile it;
	// the base class itself is never returned by ModelClasses).
	writeAndIndex("Model.php", `<?php
namespace Illuminate\Database\Eloquent;

abstract class Model {
    public function save(): bool { return true; }
}
`)

	// User model with two relation methods (no explicit return types so the
	// body-scan path is exercised, proving target recovery).
	writeAndIndex("User.php", `<?php
namespace App;

use Illuminate\Database\Eloquent\Model;

class User extends Model {
    public function posts()
    {
        return $this->hasMany(Post::class);
    }

    public function profile()
    {
        return $this->hasOne(Profile::class);
    }
}
`)

	// Related models (needed so resolveWithUses can produce FQNs in the same ns).
	writeAndIndex("Post.php", `<?php
namespace App;

use Illuminate\Database\Eloquent\Model;

class Post extends Model {}
`)

	writeAndIndex("Profile.php", `<?php
namespace App;

use Illuminate\Database\Eloquent\Model;

class Profile extends Model {}
`)

	return idx, tmpDir
}

func TestModelClasses_Laravel(t *testing.T) {
	idx, tmpDir := setupRelationsIndex(t)

	classes := ModelClasses(idx, tmpDir, "laravel")

	// Must contain App\User, App\Post, App\Profile.
	found := make(map[string]bool)
	for _, sym := range classes {
		found[sym.FQN] = true
	}

	for _, want := range []string{"App\\User", "App\\Post", "App\\Profile"} {
		if !found[want] {
			t.Errorf("ModelClasses missing %q; got %v", want, fqnList(classes))
		}
	}

	// Must NOT contain the base Eloquent Model itself.
	if found[eloquentModelFQN] {
		t.Errorf("ModelClasses must not include base %q", eloquentModelFQN)
	}
}

func TestModelClasses_UnknownFramework(t *testing.T) {
	idx, _ := setupRelationsIndex(t)
	if got := ModelClasses(idx, "", "unknown"); got != nil {
		t.Errorf("expected nil for unknown framework, got %v", fqnList(got))
	}
}

func TestModelRelations_Laravel_HasMany(t *testing.T) {
	idx, tmpDir := setupRelationsIndex(t)

	rels := ModelRelations(idx, tmpDir, "laravel")

	rel := findRelation(rels, "App\\User", "posts")
	if rel == nil {
		t.Fatalf("expected relation User->posts; relations=%v", rels)
	}

	// Target must be the fully-resolved FQN, NOT "Collection".
	if rel.TargetModel != "App\\Post" {
		t.Errorf("TargetModel = %q, want %q", rel.TargetModel, "App\\Post")
	}
	if rel.RelationType != "HasMany" {
		t.Errorf("RelationType = %q, want HasMany", rel.RelationType)
	}
	if rel.Cardinality != "many" {
		t.Errorf("Cardinality = %q, want many", rel.Cardinality)
	}
}

func TestModelRelations_Laravel_HasOne(t *testing.T) {
	idx, tmpDir := setupRelationsIndex(t)

	rels := ModelRelations(idx, tmpDir, "laravel")

	rel := findRelation(rels, "App\\User", "profile")
	if rel == nil {
		t.Fatalf("expected relation User->profile; relations=%v", rels)
	}

	if rel.TargetModel != "App\\Profile" {
		t.Errorf("TargetModel = %q, want %q", rel.TargetModel, "App\\Profile")
	}
	if rel.RelationType != "HasOne" {
		t.Errorf("RelationType = %q, want HasOne", rel.RelationType)
	}
	if rel.Cardinality != "one" {
		t.Errorf("Cardinality = %q, want one", rel.Cardinality)
	}
}

func TestModelRelations_Laravel_ExplicitReturnType(t *testing.T) {
	// Verify the explicit-return-type branch (relType from shortClassName).
	idx := symbols.NewIndex()
	tmpDir := t.TempDir()

	writeAndIndex := func(name, content string) {
		t.Helper()
		p := filepath.Join(tmpDir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		idx.IndexFile("file://"+p, content)
	}

	writeAndIndex("Model.php", `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {}
`)
	writeAndIndex("HasMany.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;
class HasMany {}
`)
	writeAndIndex("Order.php", `<?php
namespace App;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Order extends Model {
    public function items(): HasMany
    {
        return $this->hasMany(Item::class);
    }
}
`)
	writeAndIndex("Item.php", `<?php
namespace App;
use Illuminate\Database\Eloquent\Model;
class Item extends Model {}
`)

	rels := ModelRelations(idx, tmpDir, "laravel")

	rel := findRelation(rels, "App\\Order", "items")
	if rel == nil {
		t.Fatalf("expected relation Order->items; relations=%v", rels)
	}
	if rel.TargetModel != "App\\Item" {
		t.Errorf("TargetModel = %q, want App\\Item", rel.TargetModel)
	}
	if rel.RelationType != "HasMany" {
		t.Errorf("RelationType = %q, want HasMany", rel.RelationType)
	}
	if rel.Cardinality != "many" {
		t.Errorf("Cardinality = %q, want many", rel.Cardinality)
	}
}

func TestModelRelations_Sorted(t *testing.T) {
	idx, tmpDir := setupRelationsIndex(t)

	rels := ModelRelations(idx, tmpDir, "laravel")
	for i := 1; i < len(rels); i++ {
		a, b := rels[i-1], rels[i]
		if a.SourceModel > b.SourceModel {
			t.Errorf("relations not sorted by SourceModel at index %d/%d", i-1, i)
			break
		}
		if a.SourceModel == b.SourceModel && a.MethodName > b.MethodName {
			t.Errorf("relations not sorted by MethodName at index %d/%d", i-1, i)
			break
		}
	}
}

func TestModelRelations_UnknownFramework(t *testing.T) {
	idx, _ := setupRelationsIndex(t)
	if got := ModelRelations(idx, "", "unknown"); got != nil {
		t.Errorf("expected nil for unknown framework, got %v", got)
	}
}

// --- helpers ---

func findRelation(rels []Relation, source, method string) *Relation {
	for i := range rels {
		if rels[i].SourceModel == source && rels[i].MethodName == method {
			return &rels[i]
		}
	}
	return nil
}

func fqnList(syms []*symbols.Symbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.FQN
	}
	return out
}
