package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A Symfony project's Doctrine entities and the relations their ORM
// attributes declare, read the way `tusk-php introspect` and the MCP server
// list them.
func TestModelRelations_Symfony_Doctrine(t *testing.T) {
	idx := symbols.NewIndex()
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		idx.IndexFile("file://"+p, content)
	}
	write("Post.php", `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity]
class Post
{
    #[ORM\ManyToOne(targetEntity: Author::class, inversedBy: 'posts')]
    private ?Author $author = null;

    #[ORM\OneToMany(targetEntity: Comment::class, mappedBy: 'post')]
    private Collection $comments;

    #[ORM\ManyToMany(targetEntity: Tag::class)]
    private Collection $tags;

    #[ORM\OneToOne]
    private ?Cover $cover = null;

    #[ORM\Column]
    private string $title = '';
}
`)
	write("Author.php", `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity]
class Author
{
    #[ORM\OneToMany(targetEntity: Post::class, mappedBy: 'author')]
    private Collection $posts;
}
`)
	// Not an entity: no ORM\Entity attribute, so neither listed nor read.
	write("Helper.php", `<?php
namespace App\Service;

class Helper
{
    #[ORM\ManyToOne(targetEntity: Post::class)]
    private $post;
}
`)

	var classes []string
	for _, sym := range ModelClasses(idx, dir, "symfony") {
		classes = append(classes, sym.FQN)
	}
	if got := strings.Join(classes, " "); got != "App\\Entity\\Author App\\Entity\\Post" {
		t.Errorf("entities = %s", got)
	}

	var rels []string
	for _, r := range ModelRelations(idx, dir, "symfony") {
		rels = append(rels, r.SourceModel+"."+r.MethodName+" "+r.RelationType+"("+r.Cardinality+") "+r.TargetModel)
	}
	want := []string{
		"App\\Entity\\Author.posts OneToMany(many) App\\Entity\\Post",
		"App\\Entity\\Post.author ManyToOne(one) App\\Entity\\Author",
		"App\\Entity\\Post.comments OneToMany(many) App\\Entity\\Comment",
		"App\\Entity\\Post.tags ManyToMany(many) App\\Entity\\Tag",
	}
	if got := strings.Join(rels, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("relations:\n%s\nwant\n%s", got, strings.Join(want, "\n"))
	}

	if err := os.Remove(filepath.Join(dir, "Author.php")); err != nil {
		t.Fatal(err)
	}
	if got := len(ModelRelations(idx, dir, "symfony")); got != 3 {
		t.Errorf("an entity whose file is gone has no relations to read, got %d relations", got)
	}
}

// Each property reads only its own attributes: the #[ORM\Id] on $id doesn't
// make the next property read-only, and a relation right after a column is
// still a relation.
func TestDoctrineAttributesStayWithTheirProperty(t *testing.T) {
	idx, root := setupDoctrineIndex(t, `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity]
class User
{
    #[ORM\Id]
    #[ORM\GeneratedValue]
    #[ORM\Column]
    private ?int $id = null;
    #[ORM\Column]
    private string $name;
    #[ORM\ManyToOne(targetEntity: Team::class)]
    private $team;
}
`)
	AnalyzeDoctrineEntities(idx, root)
	if id := idx.Lookup("App\\Entity\\User::$id"); id == nil || !id.IsReadonly {
		t.Errorf("$id should be read-only: %+v", id)
	}
	if name := idx.Lookup("App\\Entity\\User::$name"); name == nil || name.IsReadonly {
		t.Errorf("$name took $id's #[ORM\\Id]: %+v", name)
	}
	if team := idx.Lookup("App\\Entity\\User::$team"); team == nil || !strings.Contains(team.Type, "Team") {
		t.Errorf("$team should be the Team relation: %+v", team)
	}
}

func TestDoctrineReadsAttributesFromDeclarations(t *testing.T) {
	idx, root := setupDoctrineIndex(t, `<?php
namespace App\Entity;

use Doctrine\DBAL\Types\Types;
use Doctrine\ORM\Mapping\Column;
use Doctrine\ORM\Mapping\Entity;
use Doctrine\ORM\Mapping\ManyToOne;
use Other\Column as NotDoctrine;

#[Entity(UserRepository::class)]
class User
{
    #[Column(type: Types::DATETIME_IMMUTABLE)]
    private $createdAt;
    #[Column('label', 'integer')]
    private $rank;
    #[NotDoctrine(type: 'string')]
    private $other;
    #[ManyToOne(
        targetEntity: Team::class,
    )]
    private $team;

    public function __construct(
        #[Column(type: 'boolean')]
        private $active = false,
    ) {}
}

class UserRepository {}
`)
	AnalyzeDoctrineEntities(idx, root)
	for prop, want := range map[string]string{"$createdAt": "\\DateTimeImmutable", "$rank": "int", "$active": "bool", "$team": "?App\\Entity\\Team"} {
		if sym := idx.Lookup("App\\Entity\\User::" + prop); sym == nil || sym.Type != want {
			t.Errorf("%s: %+v, want type %s", prop, sym, want)
		}
	}
	if other := idx.Lookup("App\\Entity\\User::$other"); other == nil || other.Type != "" {
		t.Errorf("$other has another library's Column: %+v", other)
	}
	if idx.Lookup("App\\Entity\\UserRepository::findOneByRank") == nil {
		t.Error("the repository named positionally gets its magic finders")
	}
}

func TestDoctrineTableNameAndAnnotations(t *testing.T) {
	file := parser.ParseFile(`<?php
namespace App\Entity;
use Doctrine\ORM\Mapping as ORM;
#[ORM\Entity]
#[ORM\Table('people')]
class Person {}
#[ORM\Entity]
#[ORM\Table(name: Person::TABLE)]
class OrderLine {}
/**
 * @ORM\Entity(repositoryClass=ThingRepository::class)
 */
class Thing {}
class NotAnEntity {}
`, token.Latest)
	var got []string
	for _, entity := range doctrineEntities(file) {
		got = append(got, entity.fqn+" "+entity.tableName()+" "+entity.repositoryClass())
	}
	want := []string{"App\\Entity\\Person people ", "App\\Entity\\OrderLine order_line ", "App\\Entity\\Thing thing App\\Entity\\ThingRepository"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q", got)
	}
}

func TestModelTableNameComesFromTheIndexedProperty(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Post.php", `<?php
namespace App\Models;
class Post { protected $table = 'blog_posts'; }
class Comment { protected $table = SOME_CONSTANT; }
class Tag {}
`)
	for fqn, want := range map[string]string{"App\\Models\\Post": "blog_posts", "App\\Models\\Comment": "comments", "App\\Models\\Tag": "tags"} {
		if got := ResolveModelTableName(idx, idx.Lookup(fqn)); got != want {
			t.Errorf("%s: %q, want %q", fqn, got, want)
		}
	}
}

func TestDoctrineSkipsWhatIsNotIndexedOrMapped(t *testing.T) {
	idx := symbols.NewIndex()
	analyzeDoctrineSource(idx, t.TempDir(), "<?php\nclass Plain {}\n")
	analyzeDoctrineSource(idx, t.TempDir(), "<?php\nuse Doctrine\\ORM\\Mapping as ORM;\n#[ORM\\Entity]\nclass NotIndexed {}\n")
	dir := t.TempDir()
	entity := "<?php\nnamespace App;\nuse Doctrine\\ORM\\Mapping as ORM;\n#[ORM\\Entity]\nclass Twice { #[ORM\\ManyToOne(targetEntity: Other::class)] private $other; }\n"
	for _, name := range []string{"a.php", "b.php"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(entity), 0o644); err != nil {
			t.Fatal(err)
		}
		idx.IndexFile("file://"+path, entity)
	}
	if got := len(ModelRelations(idx, dir, "symfony")); got != 1 {
		t.Errorf("an entity declared twice is listed once, got %d relations", got)
	}
}

func TestDoctrineEntityWithoutPropertyAttributesReadsXML(t *testing.T) {
	idx, root := setupDoctrineIndex(t, "<?php\nnamespace App\\Entity;\nuse Doctrine\\ORM\\Mapping as ORM;\n#[ORM\\Entity]\nclass User { private $name; }\n")
	analyzeDoctrineSource(idx, root, "<?php\nnamespace App\\Entity;\nuse Doctrine\\ORM\\Mapping as ORM;\n#[ORM\\Entity]\nclass User { private $name; }\n")
	injectDoctrineSchemaSource(idx, &Schema{}, "<?php\nclass Plain {}\n")
	if sym := idx.Lookup("App\\Entity\\User::$name"); sym == nil || sym.Type != "" {
		t.Errorf("no mapping leaves $name as declared: %+v", sym)
	}
}
