package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestMemberCompletionThroughStaticPropertiesArrayElementsAndUseFunction(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///lib.php", `<?php
namespace App\Support;
/** @template T */
class Box { /** @return T */ public function get() {} }
class Post { public function title(): string { return ''; } }
class Holder {
    /** @var Box<Post> */
    public static $shared;
    /** @var array<string, Box<Post>> */
    public array $boxes;
    /** @var Box<Post>[] */
    public array $legacy;
    /** @var list<Box<Post>> */
    public array $list;
    /** @var array{posts: Box<Post>, count: int} */
    public array $shape;
    public static function own(): void {
        self::$shared->get()->ti
    }
}
/** @return Box<Post> */
function postBox() {}
`)
	p := NewProvider(idx, nil, "laravel")
	prefix := "<?php\nnamespace App\\Http;\nuse App\\Support\\Box;\nuse App\\Support\\Holder;\nuse function App\\Support\\postBox;\nuse function App\\Support\\postBox as posts;\nfunction run(Holder $holder): void {\n    /** @var array<int, Box<\\App\\Support\\Post>> $local */\n    $local = load();\n    "
	for _, receiver := range []string{
		"Holder::$shared",
		"$holder->boxes['x']",
		"$holder->legacy[0]",
		"$holder->list[0]",
		"$holder->shape['posts']",
		"$local[0]",
		"postBox()",
		"posts()",
	} {
		source := prefix + receiver + "->get()->ti"
		lines := strings.Split(source, "\n")
		last := len(lines) - 1
		if !collectLabels(p.GetCompletions("file:///run.php", source, protocol.Position{Line: last, Character: len(lines[last])}))["title"] {
			t.Errorf("%s->get()-> offers Post's title()", receiver)
		}
	}
	lib := idx.GetFileSource("file:///lib.php")
	end := strings.Index(lib, "->ti\n") + len("->ti")
	pos := protocol.Position{Line: strings.Count(lib[:end], "\n"), Character: end - strings.LastIndex(lib[:end], "\n") - 1}
	if !collectLabels(p.GetCompletions("file:///lib.php", lib, pos))["title"] {
		t.Error("self::$shared->get()-> offers Post's title()")
	}
}
