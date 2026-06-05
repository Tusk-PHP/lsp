package graph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// setupModelsIndex builds an index with the same Eloquent fixture used by
// internal/models/relations_test.go:
//   - Illuminate\Database\Eloquent\Model (base, excluded from ModelClasses)
//   - App\User (hasMany(Post::class) + hasOne(Profile::class))
//   - App\Post, App\Profile
//
// All files are written to t.TempDir() and indexed via file:// URIs so that
// symbols.URIToPath + os.ReadFile work when the models package reads source.
func setupModelsIndex(t *testing.T) (*symbols.Index, string) {
	t.Helper()
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

abstract class Model {
    public function save(): bool { return true; }
}
`)

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

// TestBuildModels_Kind verifies the graph Kind is "models".
func TestBuildModels_Kind(t *testing.T) {
	idx, root := setupModelsIndex(t)

	g := BuildModels(idx, root, "laravel", Options{Deps: DepsFull})

	if g.Kind != "models" {
		t.Errorf("Kind = %q, want %q", g.Kind, "models")
	}
	if g.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", g.SchemaVersion, SchemaVersion)
	}
}

// TestBuildModels_Nodes verifies that model nodes exist for App\User, App\Post,
// and App\Profile and that their Kind is "model".
func TestBuildModels_Nodes(t *testing.T) {
	idx, root := setupModelsIndex(t)

	g := BuildModels(idx, root, "laravel", Options{Deps: DepsFull})

	nodeMap := map[string]Node{}
	for _, n := range g.Nodes {
		nodeMap[n.ID] = n
	}

	for _, fqn := range []string{"App\\User", "App\\Post", "App\\Profile"} {
		n, ok := nodeMap[fqn]
		if !ok {
			t.Errorf("missing node for %q", fqn)
			continue
		}
		if n.Kind != "model" {
			t.Errorf("node %q Kind = %q, want %q", fqn, n.Kind, "model")
		}
	}
}

// TestBuildModels_Edges verifies the HasMany and HasOne relation edges.
func TestBuildModels_Edges(t *testing.T) {
	idx, root := setupModelsIndex(t)

	g := BuildModels(idx, root, "laravel", Options{Deps: DepsFull})

	type edgeKey struct{ from, to, kind string }
	edgeSet := map[edgeKey]struct{}{}
	for _, e := range g.Edges {
		edgeSet[edgeKey{e.From, e.To, e.Kind}] = struct{}{}
	}

	wants := []struct {
		from, to, kind string
	}{
		{"App\\User", "App\\Post", "HasMany"},
		{"App\\User", "App\\Profile", "HasOne"},
	}
	for _, w := range wants {
		if _, ok := edgeSet[edgeKey{w.from, w.to, w.kind}]; !ok {
			t.Errorf("missing edge {%s -> %s, kind=%q}; got edges %v", w.from, w.to, w.kind, g.Edges)
		}
	}
}

// TestBuildModels_NilIdx verifies that a nil index returns an empty graph
// without panicking.
func TestBuildModels_NilIdx(t *testing.T) {
	g := BuildModels(nil, "", "laravel", Options{})

	if g == nil {
		t.Fatal("BuildModels(nil, ...) returned nil")
	}
	if g.Kind != "models" {
		t.Errorf("Kind = %q, want %q", g.Kind, "models")
	}
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes for nil idx, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges for nil idx, got %d", len(g.Edges))
	}
}

// TestBuildModels_NodeDedup verifies that each model FQN appears as exactly
// one node even when it is referenced both as a class and as a relation target.
func TestBuildModels_NodeDedup(t *testing.T) {
	idx, root := setupModelsIndex(t)

	g := BuildModels(idx, root, "laravel", Options{Deps: DepsFull})

	seen := map[string]int{}
	for _, n := range g.Nodes {
		seen[n.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("node %q appears %d times (want exactly 1)", id, count)
		}
	}
}

// TestBuildModels_Sorted verifies that nodes and edges are sorted after Build.
func TestBuildModels_Sorted(t *testing.T) {
	idx, root := setupModelsIndex(t)

	g := BuildModels(idx, root, "laravel", Options{Deps: DepsFull})

	for i := 1; i < len(g.Nodes); i++ {
		if g.Nodes[i-1].ID > g.Nodes[i].ID {
			t.Errorf("nodes not sorted at index %d/%d: %q > %q",
				i-1, i, g.Nodes[i-1].ID, g.Nodes[i].ID)
		}
	}
	for i := 1; i < len(g.Edges); i++ {
		a, b := g.Edges[i-1], g.Edges[i]
		less := a.From < b.From ||
			(a.From == b.From && a.To < b.To) ||
			(a.From == b.From && a.To == b.To && a.Kind <= b.Kind)
		if !less {
			t.Errorf("edges not sorted at index %d/%d: {%s->%s %s} > {%s->%s %s}",
				i-1, i, a.From, a.To, a.Kind, b.From, b.To, b.Kind)
		}
	}
}
