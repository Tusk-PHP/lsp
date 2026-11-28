package symbols

import "testing"

const refinedEntity = `<?php
namespace App;
class Post {
    private \Doctrine\Common\Collections\Collection $comments;
    private string $title;
    private $body;
    const tags = 1;
    public function author() { return 1; }
    public function editor(): User { return 1; }
}
`

func TestRefinePropertyTakesTheMappedTypeWhereItSaysMore(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///post.php", refinedEntity)
	for _, tc := range []struct{ property, mapped, want string }{
		{"$comments", `Doctrine\Common\Collections\Collection<int, App\Comment>`, `Doctrine\Common\Collections\Collection<int, App\Comment>`},
		{"$title", "int", "string"},
		{"$body", "?string", "?string"},
	} {
		fqn := `App\Post::` + tc.property
		if !idx.RefineProperty(fqn, tc.mapped, false) {
			t.Fatalf("%s isn't indexed", fqn)
		}
		if got := idx.Lookup(fqn).Type; got != tc.want {
			t.Errorf("%s mapped as %s: type %q, want %q", tc.property, tc.mapped, got, tc.want)
		}
	}
	if idx.RefineProperty(`App\Post::$missing`, "int", false) {
		t.Error("a property the class doesn't declare was refined")
	}
}

func TestRefineMethodReturnTypeFillsOnlyAMissingNativeType(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///post.php", refinedEntity)
	idx.RefineMethodReturnType(`App\Post::author`, `BelongsTo<App\User, App\Post>`)
	idx.RefineMethodReturnType(`App\Post::editor`, `BelongsTo<App\User, App\Post>`)
	idx.RefineMethodReturnType(`App\Post::tags`, "int")
	idx.RefineMethodReturnType(`App\Post::missing`, "int")
	if got := idx.Lookup(`App\Post::author`).ReturnType; got != `BelongsTo<App\User, App\Post>` {
		t.Errorf("untyped author() returns %q, want the refined type", got)
	}
	if got := idx.Lookup(`App\Post::editor`).ReturnType; got != `App\User` {
		t.Errorf("editor(): User returns %q, want its declared type", got)
	}
	if got := idx.Lookup(`App\Post::tags`); got == nil || got.Kind != KindConstant || got.ReturnType != "" {
		t.Errorf("the constant tags was changed: %+v", got)
	}
	if idx.Lookup(`App\Post::missing`) != nil {
		t.Error("refining a method the class doesn't declare added one")
	}
}
