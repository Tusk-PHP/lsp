package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestUnknownClassIgnoresKeywordLikeVariables(t *testing.T) {
	idx := symbols.NewIndex()
	src := `<?php
function diff( $new, $old, $extends, $catch, $implements ) {
    foreach ( $new as $new_key => $new_data ) {}
    foreach ( $old as $old_data ) {}
    $x = $extends instanceof Countable;
    $y = $this->new as;
    $z = $obj->extends and $obj->implements or $catch ( 1 );
    $w = Foo::new;
    $m = new Missing();
}
`
	idx.IndexFile("file:///lib.php", "<?php\nclass Foo { const new = 1; }\ninterface Countable {}\n")
	got := (&UnknownClassRule{}).Check(parser.ParseFile(src), src, idx)
	if len(got) != 1 || got[0].Message != "Unknown class 'Missing'" {
		t.Errorf("only Missing should be reported, got %+v", got)
	}
}
