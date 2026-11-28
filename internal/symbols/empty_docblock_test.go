package symbols

import (
	"strings"
	"testing"
)

func TestIsDeprecatedReadsAnEmptyDocBlock(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///classes.php", "<?php\n/**\n */\ninterface Plain {}\n/**\n * @deprecated\n */\nclass Old {}\n")
	if plain := idx.Lookup("Plain"); plain == nil || plain.IsDeprecated() {
		t.Errorf("an interface with an empty doc block: %+v, want it indexed and not deprecated", plain)
	}
	if old := idx.Lookup("Old"); old == nil || !old.IsDeprecated() {
		t.Errorf("a class with @deprecated: %+v, want it deprecated", old)
	}
}

func TestIDEHelperMergeReadsEachDocBlock(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///app/User.php", "<?php\nclass User { public function save(): bool { return true; } }\nclass Post {}\n")
	idx.IndexIDEHelperFile("file:///_ide_helper.php", `<?php
/**
 * @property int
 * @property string $name
 * @method ()
 * @method bool save()
 * @method static self where(string $column, mixed $value)
 */
class User {}
/**
 */
class Post {}
`)
	var names []string
	for _, child := range idx.Lookup("User").Children {
		names = append(names, child.Name)
	}
	if got := strings.Join(names, ","); got != "save,$name,where" {
		t.Errorf("User's members = %s, want save,$name,where", got)
	}
	if where := idx.Lookup("User::where"); where == nil || len(where.Params) != 2 || where.Params[0].Name != "$column" {
		t.Errorf("User::where = %+v, want its two parameters", where)
	}
	if save := idx.Lookup("User::save"); save == nil || save.IsVirtual {
		t.Errorf("User::save = %+v, want the declared method kept", save)
	}
	if post := idx.Lookup("Post"); post == nil || len(post.Children) != 0 {
		t.Errorf("Post with an empty doc block = %+v, want no members", post)
	}
}
