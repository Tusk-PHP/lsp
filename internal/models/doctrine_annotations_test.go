package models

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestFindDocAnnotation(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		find string
		args parser.AttributeArgs
		ok   bool
	}{
		{"bare", "/** @ORM\\Entity */", `ORM\Entity`, nil, true},
		{"after the opening stars", "/**@Entity*/", "Entity", nil, true},
		{"at the end", "@Entity", "Entity", nil, true},
		{"named and positional", `/**
 * Summary, not an @ annotation.
 * @ORM\Table("people", schema="hr")
 * @ORM\Entity(repositoryClass=PersonRepository::class, readOnly=true)
 */`, `ORM\Entity`, parser.AttributeArgs{{Name: "repositoryClass", Value: "PersonRepository::class"}, {Name: "readOnly", Value: "true"}}, true},
		{"positional", `/** @ORM\Table("people", schema="hr") */`, `ORM\Table`, parser.AttributeArgs{{Value: `"people"`}, {Name: "schema", Value: `"hr"`}}, true},
		{"over several lines", `/**
 * @ORM\Entity(
 *     repositoryClass = \App\Repository\PersonRepository::class,
 * )
 */`, `ORM\Entity`, parser.AttributeArgs{{Name: "repositoryClass", Value: `\App\Repository\PersonRepository::class`}}, true},
		{"nested values and quotes", `/** @ORM\Table(name="a ""b"", c)", indexes={@ORM\Index(columns={"x", "y"})}, options={"n": -1.5}) */`, `ORM\Table`,
			parser.AttributeArgs{
				{Name: "name", Value: `"a ""b"", c)"`},
				{Name: "indexes", Value: `{@ORM\Index(columns={"x","y"})}`},
				{Name: "options", Value: `{"n":-1.5}`},
			}, true},
		{"nested annotations are not top-level", `/** @ORM\Table(indexes={@ORM\Index(columns={"x"})}) */`, `ORM\Index`, nil, false},
		{"skipped arguments", `/** @ORM\Table(name=")") @ORM\Entity */`, `ORM\Entity`, nil, true},
		{"not after whitespace", "/** mail me at someone@Entity.example */", "Entity", nil, false},
		{"no name", "/** @ (x) @1 */", "1", nil, false},
		{"a space before the parenthesis", "/** @Entity (repositoryClass=X::class) */", "Entity", nil, true},
		{"unterminated arguments", "/** @Entity(repositoryClass=X::class */ ", "Entity", nil, false},
		{"unterminated string", `/** @Entity(name="x) */`, "Entity", nil, false},
		{"unterminated at the end", `@Entity(name=x`, "Entity", nil, false},
		{"unexpected character", "/** @Entity(name='x') @Table */", "Entity", nil, false},
		{"after unreadable arguments", "/** @Entity(name='x') @Table */", "Table", nil, true},
		{"after skipped unreadable arguments", "/** @Other(name='x') @Entity */", "Entity", nil, true},
		{"empty arguments", "/** @Entity() */", "Entity", nil, true},
		{"empty argument", "/** @Table(,a) */", "Table", parser.AttributeArgs{{Value: "a"}}, true},
		{"absent", "/** @Table */", "Entity", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args, ok := findDocAnnotation(c.doc, func(name string) bool { return name == c.find })
			if ok != c.ok || !reflect.DeepEqual(args, c.args) {
				t.Fatalf("got %#v, %v; want %#v, %v", args, ok, c.args, c.ok)
			}
		})
	}
}

func TestDoctrineEntityAnnotations(t *testing.T) {
	file := parser.ParseFile(`<?php
namespace App\Entity;
use Doctrine\ORM\Mapping as ORM;
use Doctrine\ORM\Mapping\Entity;
/**
 * @ORM\Entity(
 *     repositoryClass=PersonRepository::class
 * )
 */
class Person {}
/** @Entity(readOnly=true, repositoryClass=\Other\LogRepository::class) */
class Log {}
/** @ORM\Entity(repositoryClass="App\Repository\NoteRepository") */
class Note {}
/** Written by someone@ORM\Entity.example; see @ORM\Table. */
class Plain {}
/** @Other\Entity */
class Unrelated {}
`, token.Latest)
	var got []string
	for _, entity := range doctrineEntities(file) {
		got = append(got, entity.fqn+" "+entity.repository)
	}
	want := []string{`App\Entity\Person App\Entity\PersonRepository`, `App\Entity\Log Other\LogRepository`, `App\Entity\Note `}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func BenchmarkDoctrineEntities(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\nuse Doctrine\\ORM\\Mapping as ORM;\n")
	for i := range 200 {
		if i%4 == 0 {
			fmt.Fprintf(&src, "/**\n * An entity.\n * @ORM\\Entity(repositoryClass=Repo%d::class)\n */\nclass Entity%d {}\n", i, i)
			continue
		}
		fmt.Fprintf(&src, "/**\n * Model %d, see https://example.com/a@b.\n * @property int $id\n * @method static self find(int $id)\n */\nclass Model%d {}\n", i, i)
	}
	file := parser.ParseFile(src.String(), token.Latest)
	b.ReportAllocs()
	for b.Loop() {
		if got := len(doctrineEntities(file)); got != 50 {
			b.Fatalf("%d entities", got)
		}
	}
}
