package models

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const reanalyzeBaseModel = `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {}
`

const reanalyzePost = `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Post extends Model
{
    public function comments(): HasMany
    {
        return $this->hasMany(Comment::class);
    }

    public function getSlugAttribute(): string
    {
        return '';
    }
}
`

func hasMember(idx *symbols.Index, classFQN, name string) bool {
	for _, m := range idx.GetClassMembers(classFQN) {
		if m.Name == name {
			return true
		}
	}
	return false
}

// TestAnalyzeModelFileRestoresWhatReindexingDrops re-indexes a model file and
// checks that re-analysing that file gives the model back its relation,
// accessor, schema columns, migration columns and builder methods.
func TestAnalyzeModelFileRestoresWhatReindexingDrops(t *testing.T) {
	const uri = "file:///app/Models/Post.php"
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", reanalyzeBaseModel)
	idx.IndexFile(uri, reanalyzePost)

	schema := &Schema{Source: SchemaSourceLive, Tables: []SchemaTable{{
		Name:    "posts",
		Columns: []SchemaColumn{{Name: "id", DataType: "bigint", ColumnType: "bigint(20)"}, {Name: "title", DataType: "varchar", ColumnType: "varchar(255)"}},
	}}}
	migrations := map[string]map[string]*MigrationColumn{
		"posts": {"published_at": {Name: "published_at", Type: "\\DateTimeInterface", Nullable: true}},
	}
	want := []string{"$comments", "$slug", "$id", "$title", "$published_at", "withTrashed"}

	AnalyzeModelFile(idx, "/", "laravel", uri, reanalyzePost, schema, migrations)
	for _, name := range want {
		if !hasMember(idx, `App\Models\Post`, name) {
			t.Errorf("first analysis: Post has no %s", name)
		}
	}

	idx.IndexFile(uri, reanalyzePost)
	if hasMember(idx, `App\Models\Post`, "$title") {
		t.Fatal("re-indexing kept an injected column; this test no longer covers the bug")
	}
	AnalyzeModelFile(idx, "/", "laravel", uri, reanalyzePost, schema, migrations)
	for _, name := range want {
		if !hasMember(idx, `App\Models\Post`, name) {
			t.Errorf("after re-indexing: Post has no %s", name)
		}
	}
	if doc := idx.Lookup(`App\Models\Post::$published_at`); doc == nil || doc.DocComment != "From migration" {
		t.Errorf("migration column: got %+v", doc)
	}
}

func TestAnalyzeModelFileUsesTheTableProperty(t *testing.T) {
	const uri = "file:///app/Models/Legacy.php"
	src := `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Legacy extends Model
{
    protected $table = 'tbl_legacy';
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", reanalyzeBaseModel)
	idx.IndexFile(uri, src)
	schema := &Schema{Source: SchemaSourceLive, Tables: []SchemaTable{{Name: "tbl_legacy", Columns: []SchemaColumn{{Name: "code", DataType: "varchar"}}}}}
	AnalyzeModelFile(idx, "/", "laravel", uri, src, schema, nil)
	if !hasMember(idx, `App\Models\Legacy`, "$code") {
		t.Error("Legacy has no $code from its tbl_legacy table")
	}
}

func TestAnalyzeModelFileLeavesOtherClassesAlone(t *testing.T) {
	const uri = "file:///app/Support/Helper.php"
	src := `<?php
namespace App\Support;
class Helper {}
`
	idx := symbols.NewIndex()
	idx.IndexFile(uri, src)
	schema := &Schema{Source: SchemaSourceLive, Tables: []SchemaTable{{Name: "helpers", Columns: []SchemaColumn{{Name: "id"}}}}}
	AnalyzeModelFile(idx, "/", "laravel", uri, src, schema, nil)
	if len(idx.GetClassMembers(`App\Support\Helper`)) != 0 {
		t.Error("a class that isn't a model got members")
	}
}

func TestAnalyzeModelFileDoctrineEntity(t *testing.T) {
	const uri = "file:///src/Entity/User.php"
	src := `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity]
#[ORM\Table(name: 'users')]
class User
{
    #[ORM\Column(type: 'string')]
    private string $name;
}
`
	idx := symbols.NewIndex()
	idx.IndexFile(uri, src)
	schema := &Schema{Source: SchemaSourceLive, Tables: []SchemaTable{{Name: "users", Columns: []SchemaColumn{{Name: "legacy_flag", DataType: "tinyint"}}}}}
	AnalyzeModelFile(idx, "/", "symfony", uri, src, schema, nil)
	if !hasMember(idx, `App\Entity\User`, "$legacy_flag") {
		t.Error("User has no $legacy_flag from the users table")
	}
}
