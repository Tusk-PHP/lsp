package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/symbols/symboltest"
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

	edited := reanalyzePost + "\n"
	idx.IndexFile(uri, edited)
	if hasMember(idx, `App\Models\Post`, "$title") {
		t.Fatal("re-indexing an edit kept an injected column; this test no longer covers the bug")
	}
	AnalyzeModelFile(idx, "/", "laravel", uri, edited, schema, migrations)
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

const pinnedDoctrineUser = `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;

#[Pinned] function pinned() {} /*
#[ORM\Entity]
#[ORM\Table(name: 'users')]
class User
{
    #[ORM\Column(type: 'boolean')]
    private $active;
}
// */
`

func legacyUsersSchema() *Schema {
	return &Schema{Source: SchemaSourceLive, Tables: []SchemaTable{{Name: "users", Columns: []SchemaColumn{{Name: "legacy_flag", DataType: "tinyint"}}}}}
}

func TestAnalyzeModelFileReadsADoctrineEntityWithTheIndexsGrammar(t *testing.T) {
	const uri = "file:///src/Entity/User.php"
	idx := symbols.NewIndex()
	symboltest.PinToPHP74(t, idx, uri, pinnedDoctrineUser)
	AnalyzeModelFile(idx, "/", "symfony", uri, pinnedDoctrineUser, legacyUsersSchema(), nil)

	if sym := idx.Lookup(`App\Entity\User::$active`); sym == nil || sym.Type != "bool" {
		t.Errorf("$active is %+v, want the boolean column of the class PHP 7.4 reads, where it is code and not a comment", sym)
	}
	if !hasMember(idx, `App\Entity\User`, "$legacy_flag") {
		t.Error("User has no $legacy_flag from the users table of the class PHP 7.4 reads")
	}
}

func TestSymfonySchemaColumnsReachTheEntityTheIndexsGrammarReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "src", "Entity", "User.php")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(pinnedDoctrineUser), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	symboltest.PinToPHP74(t, idx, symbols.PathToURI(path), pinnedDoctrineUser)
	injectSymfonySchemaProperties(idx, legacyUsersSchema(), filepath.Dir(path))

	if !hasMember(idx, `App\Entity\User`, "$legacy_flag") {
		t.Error("User has no $legacy_flag from the users table of the class PHP 7.4 reads")
	}
}
