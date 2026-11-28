package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		idx.IndexFile(symbols.PathToURI(p), content)
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

	// An entity whose file is gone has no relations to read.
	if got := extractDoctrineRelations(&symbols.Symbol{FQN: "App\\Entity\\Gone", URI: symbols.PathToURI(filepath.Join(dir, "Gone.php"))}); got != nil {
		t.Errorf("missing file gave %v", got)
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

func TestPropertyAttributeBlock(t *testing.T) {
	lines := strings.Split(`class A
{
    #[ORM\Id]
    private $id;
    #[ORM\Column]
    #[Assert\NotBlank]
    private $name;
}`, "\n")
	if got := propertyAttributeBlock(lines, 6, 0); got != "    #[ORM\\Column]\n    #[Assert\\NotBlank]\n    private $name;" {
		t.Errorf("block for $name = %q", got)
	}
	if got := propertyAttributeBlock(lines, 3, 0); got != "    #[ORM\\Id]\n    private $id;" {
		t.Errorf("block for $id = %q", got)
	}
	if got := propertyAttributeBlock(lines, 3, 3); got != "    private $id;" {
		t.Errorf("block at the class's first line = %q", got)
	}
}
