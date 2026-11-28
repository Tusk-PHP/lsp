package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func setupDoctrineIndex(t *testing.T, entitySource string) (*symbols.Index, string) {
	t.Helper()
	idx := symbols.NewIndex()

	tmpDir := t.TempDir()
	entityPath := filepath.Join(tmpDir, "User.php")
	if err := os.WriteFile(entityPath, []byte(entitySource), 0644); err != nil {
		t.Fatal(err)
	}

	idx.IndexFile("file://"+entityPath, entitySource)

	return idx, tmpDir
}

func TestDoctrineColumnAttributes(t *testing.T) {
	source := `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity]
#[ORM\Table(name: 'users')]
class User
{
    #[ORM\Id]
    #[ORM\GeneratedValue]
    #[ORM\Column(type: 'integer')]
    private ?int $id = null;

    #[ORM\Column(type: 'string', length: 255)]
    private string $name;

    #[ORM\Column(type: 'string')]
    private string $email;

    #[ORM\Column(type: 'datetime')]
    private ?\DateTimeInterface $createdAt = null;

    #[ORM\Column(type: 'boolean')]
    private bool $active = true;

    #[ORM\Column(type: 'json')]
    private array $metadata = [];
}
`
	idx, rootPath := setupDoctrineIndex(t, source)
	AnalyzeDoctrineEntities(idx, rootPath)

	t.Run("column type inferred from PHP type hint", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\User::$name")
		if sym == nil {
			t.Fatal("expected property '$name'")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string', got %q", sym.Type)
		}
	})

	t.Run("existing typed properties preserved", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\User::$email")
		if sym == nil {
			t.Fatal("expected property '$email'")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string', got %q", sym.Type)
		}
	})

	t.Run("datetime column type", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\User::$createdAt")
		if sym == nil {
			t.Fatal("expected property '$createdAt'")
		}
		// Has PHP type hint, should use it
		if sym.Type == "" {
			t.Error("expected non-empty type")
		}
	})
}

func TestDoctrineRelationAttributes(t *testing.T) {
	source := `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;
use Doctrine\Common\Collections\Collection;

#[ORM\Entity]
class Post
{
    #[ORM\Id]
    #[ORM\Column(type: 'integer')]
    private ?int $id = null;

    #[ORM\ManyToOne(targetEntity: User::class)]
    private ?User $author = null;

    #[ORM\OneToMany(targetEntity: Comment::class, mappedBy: 'post')]
    private Collection $comments;

    #[ORM\ManyToMany(targetEntity: Tag::class)]
    private Collection $tags;

    #[ORM\OneToOne(targetEntity: PostMeta::class)]
    private ?PostMeta $meta = null;
}
`
	idx, rootPath := setupDoctrineIndex(t, source)

	// Index related entities
	idx.IndexFile("file:///User.php", `<?php
namespace App\Entity;
class User {}
`)
	idx.IndexFile("file:///Comment.php", `<?php
namespace App\Entity;
class Comment {}
`)
	idx.IndexFile("file:///Tag.php", `<?php
namespace App\Entity;
class Tag {}
`)
	idx.IndexFile("file:///PostMeta.php", `<?php
namespace App\Entity;
class PostMeta {}
`)

	AnalyzeDoctrineEntities(idx, rootPath)

	t.Run("ManyToOne creates nullable property", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Post::$author")
		if sym == nil {
			t.Fatal("expected property '$author'")
		}
		// Has PHP type hint ?User, should preserve it
		if sym.Type == "" {
			t.Error("expected non-empty type")
		}
	})

	t.Run("OneToMany creates Collection property", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Post::$comments")
		if sym == nil {
			t.Fatal("expected property '$comments'")
		}
	})

	t.Run("ManyToMany creates Collection property", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Post::$tags")
		if sym == nil {
			t.Fatal("expected property '$tags'")
		}
	})

	t.Run("OneToOne creates nullable property", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Post::$meta")
		if sym == nil {
			t.Fatal("expected property '$meta'")
		}
	})
}

func TestDoctrineRepositoryMagicMethods(t *testing.T) {
	source := `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity(repositoryClass: UserRepository::class)]
class User
{
    #[ORM\Id]
    #[ORM\Column(type: 'integer')]
    private ?int $id = null;

    #[ORM\Column(type: 'string')]
    private string $email;

    #[ORM\Column(type: 'string')]
    private string $name;
}
`
	idx, rootPath := setupDoctrineIndex(t, source)

	// Index the repository class
	idx.IndexFile("file:///UserRepository.php", `<?php
namespace App\Entity;

use Doctrine\ORM\EntityRepository;

class UserRepository extends EntityRepository
{
}
`)

	AnalyzeDoctrineEntities(idx, rootPath)

	t.Run("base find method injected", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\UserRepository::find")
		if sym == nil {
			t.Fatal("expected method 'find'")
		}
		if !sym.IsVirtual {
			t.Error("expected virtual")
		}
		if sym.ReturnType != "?App\\Entity\\User" {
			t.Errorf("expected return type '?App\\Entity\\User', got %q", sym.ReturnType)
		}
	})

	t.Run("findAll injected", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\UserRepository::findAll")
		if sym == nil {
			t.Fatal("expected method 'findAll'")
		}
		if sym.ReturnType != "array" {
			t.Errorf("expected return type 'array', got %q", sym.ReturnType)
		}
	})

	t.Run("findByEmail injected", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\UserRepository::findByEmail")
		if sym == nil {
			t.Fatal("expected method 'findByEmail'")
		}
		if sym.ReturnType != "array" {
			t.Errorf("expected return type 'array', got %q", sym.ReturnType)
		}
	})

	t.Run("findOneByEmail injected", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\UserRepository::findOneByEmail")
		if sym == nil {
			t.Fatal("expected method 'findOneByEmail'")
		}
		if sym.ReturnType != "?App\\Entity\\User" {
			t.Errorf("expected return type '?App\\Entity\\User', got %q", sym.ReturnType)
		}
	})

	t.Run("countByName injected", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\UserRepository::countByName")
		if sym == nil {
			t.Fatal("expected method 'countByName'")
		}
		if sym.ReturnType != "int" {
			t.Errorf("expected return type 'int', got %q", sym.ReturnType)
		}
	})
}

func TestDoctrineXMLMapping(t *testing.T) {
	idx := symbols.NewIndex()
	tmpDir := t.TempDir()

	// Create entity class without attributes
	entitySource := `<?php
namespace App\Entity;

class Product
{
}
`
	entityPath := filepath.Join(tmpDir, "Product.php")
	if err := os.WriteFile(entityPath, []byte(entitySource), 0644); err != nil {
		t.Fatal(err)
	}
	idx.IndexFile("file://"+entityPath, entitySource)

	// Create XML mapping
	xmlDir := filepath.Join(tmpDir, "config", "doctrine")
	if err := os.MkdirAll(xmlDir, 0755); err != nil {
		t.Fatal(err)
	}
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<doctrine-mapping xmlns="http://doctrine-project.org/schemas/orm/doctrine-mapping">
    <entity name="App\Entity\Product" table="products">
        <id name="id" type="integer">
            <generator strategy="AUTO"/>
        </id>
        <field name="title" type="string" column="title"/>
        <field name="price" type="decimal" column="price"/>
        <field name="description" type="text"/>
        <one-to-many field="reviews" target-entity="App\Entity\Review" mapped-by="product"/>
        <many-to-one field="category" target-entity="App\Entity\Category"/>
    </entity>
</doctrine-mapping>`
	xmlPath := filepath.Join(xmlDir, "Product.orm.xml")
	if err := os.WriteFile(xmlPath, []byte(xmlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// We need to make the file look like a Doctrine entity for the analyzer.
	// The XML fallback only runs if no PHP attributes are found, but the entity
	// must still be detected. Since the entity has no #[ORM\Entity], we test
	// the XML parser directly.
	entitySym := idx.Lookup("App\\Entity\\Product")
	if entitySym == nil {
		t.Fatal("expected entity symbol")
	}

	resolve := func(name string) string { return name }
	parseDoctrineXMLMapping(idx, entitySym, "App\\Entity\\Product", xmlPath, resolve)

	t.Run("XML id field", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Product::$id")
		if sym == nil {
			t.Fatal("expected virtual property '$id'")
		}
		if sym.Type != "int" {
			t.Errorf("expected type 'int', got %q", sym.Type)
		}
		if !sym.IsReadonly {
			t.Error("expected readonly for id field")
		}
	})

	t.Run("XML string field", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Product::$title")
		if sym == nil {
			t.Fatal("expected virtual property '$title'")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string', got %q", sym.Type)
		}
	})

	t.Run("XML decimal field", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Product::$price")
		if sym == nil {
			t.Fatal("expected virtual property '$price'")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string' for decimal, got %q", sym.Type)
		}
	})

	t.Run("XML text field", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Product::$description")
		if sym == nil {
			t.Fatal("expected virtual property '$description'")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string' for text, got %q", sym.Type)
		}
	})

	t.Run("XML one-to-many relation", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Product::$reviews")
		if sym == nil {
			t.Fatal("expected virtual property '$reviews'")
		}
		if sym.Type != "Doctrine\\Common\\Collections\\Collection<int, App\\Entity\\Review>" {
			t.Errorf("expected Collection type, got %q", sym.Type)
		}
	})

	t.Run("XML many-to-one relation", func(t *testing.T) {
		sym := idx.Lookup("App\\Entity\\Product::$category")
		if sym == nil {
			t.Fatal("expected virtual property '$category'")
		}
		if !strings.Contains(sym.Type, "Category") || !strings.HasPrefix(sym.Type, "?") {
			t.Errorf("expected nullable Category type, got %q", sym.Type)
		}
	})
}

func TestDoctrineTypeMapping(t *testing.T) {
	tests := []struct {
		doctrineType string
		phpType      string
	}{
		{"string", "string"},
		{"text", "string"},
		{"integer", "int"},
		{"smallint", "int"},
		{"bigint", "int"},
		{"boolean", "bool"},
		{"decimal", "string"},
		{"float", "float"},
		{"datetime", "\\DateTimeInterface"},
		{"datetime_immutable", "\\DateTimeImmutable"},
		{"date", "\\DateTimeInterface"},
		{"json", "array"},
		{"binary", "string"},
		{"guid", "string"},
	}
	for _, tt := range tests {
		if got, ok := doctrineTypeMap[tt.doctrineType]; !ok || got != tt.phpType {
			t.Errorf("doctrineTypeMap[%q] = %q, want %q", tt.doctrineType, got, tt.phpType)
		}
	}
}

func TestDoctrineAnalysisRefinesPropertiesWhileTheyAreRead(t *testing.T) {
	source := `<?php
namespace App\Entity;

use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity]
class Tag
{
    #[ORM\Id]
    #[ORM\Column(type: 'integer')]
    private $id;

    #[ORM\ManyToOne(targetEntity: Tag::class)]
    private $parent;
}
`
	idx, rootPath := setupDoctrineIndex(t, source)
	id := idx.Lookup(`App\Entity\Tag::$id`)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = id.Type + idx.Lookup(`App\Entity\Tag::$parent`).Type
			_ = id.IsReadonly
		}
	}()
	AnalyzeDoctrineEntities(idx, rootPath)
	<-done

	if got := idx.Lookup(`App\Entity\Tag::$id`); got == nil || got.Type != "int" || !got.IsReadonly {
		t.Errorf("the column gets its type and stays readonly: %+v", got)
	}
	if got := idx.Lookup(`App\Entity\Tag::$parent`); got == nil || got.Type != `?App\Entity\Tag` {
		t.Errorf("the relation gets its type: %+v", got)
	}
	members := 0
	for _, m := range idx.GetClassMembers(`App\Entity\Tag`) {
		if m.Name == "$id" {
			members++
			if m.Type != "int" {
				t.Errorf("the class lists the refined property: %+v", m)
			}
		}
	}
	if members != 1 {
		t.Errorf("the property is listed once: %d", members)
	}
}

func TestDoctrineToManyPropertiesCarryTheTargetEntity(t *testing.T) {
	idx, root := setupDoctrineIndex(t, `<?php
namespace App\Entity;

use Doctrine\Common\Collections\Collection;
use Doctrine\ORM\Mapping as ORM;

#[ORM\Entity]
class Post
{
    #[ORM\OneToMany(targetEntity: Comment::class, mappedBy: 'post')]
    private Collection $comments;
    #[ORM\ManyToMany(targetEntity: Tag::class)]
    private $tags;
    #[ORM\ManyToMany(targetEntity: Tag::class)]
    private iterable $iterableTags;
    #[ORM\OneToMany(mappedBy: 'post')]
    private Collection $untargeted;
}
`)
	AnalyzeDoctrineEntities(idx, root)
	for prop, want := range map[string]string{
		"$comments":     `Doctrine\Common\Collections\Collection<int, App\Entity\Comment>`,
		"$tags":         `Doctrine\Common\Collections\Collection<int, App\Entity\Tag>`,
		"$iterableTags": "iterable",
		"$untargeted":   `Doctrine\Common\Collections\Collection`,
	} {
		if sym := idx.Lookup(`App\Entity\Post::` + prop); sym == nil || sym.Type != want {
			t.Errorf("%s: %+v, want type %s", prop, sym, want)
		}
	}
}

func TestDoctrineXMLMappingRefinesDeclaredProperties(t *testing.T) {
	idx, dir := setupDoctrineIndex(t, `<?php
namespace App\Entity;

use Doctrine\Common\Collections\Collection;

class Product
{
    private $id;
    private ?string $title;
    private Collection $reviews;
    private $tags;
    private $category;
    private $cover;
    private $location;
}
`)
	xmlPath := filepath.Join(dir, "Product.orm.xml")
	if err := os.WriteFile(xmlPath, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<doctrine-mapping xmlns="http://doctrine-project.org/schemas/orm/doctrine-mapping">
    <entity name="App\Entity\Product">
        <id name="id" type="integer"/>
        <field name="title" type="string"/>
        <one-to-many field="reviews" target-entity="App\Entity\Review" mapped-by="product"/>
        <many-to-many field="tags" target-entity="App\Entity\Tag"/>
        <many-to-one field="category" target-entity="App\Entity\Category"/>
        <one-to-one field="cover" target-entity="App\Entity\Image"/>
        <field name="location" type="geometry"/>
    </entity>
</doctrine-mapping>`), 0o644); err != nil {
		t.Fatal(err)
	}
	parseDoctrineXMLMapping(idx, idx.Lookup(`App\Entity\Product`), `App\Entity\Product`, xmlPath, func(name string) string { return name })
	for prop, want := range map[string]string{
		"$id":       "int",
		"$title":    "?string",
		"$reviews":  `Doctrine\Common\Collections\Collection<int, App\Entity\Review>`,
		"$tags":     `Doctrine\Common\Collections\Collection<int, App\Entity\Tag>`,
		"$category": `?App\Entity\Category`,
		"$cover":    `?App\Entity\Image`,
		"$location": "mixed",
	} {
		if sym := idx.Lookup(`App\Entity\Product::` + prop); sym == nil || sym.Type != want {
			t.Errorf("%s: %+v, want type %s", prop, sym, want)
		}
	}
	if id := idx.Lookup(`App\Entity\Product::$id`); id == nil || !id.IsReadonly {
		t.Errorf("the mapped id is read-only: %+v", id)
	}
}
