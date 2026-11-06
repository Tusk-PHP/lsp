package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// WordPress's ftp_base calls methods that only its subclasses ftp_sockets and
// ftp_pure declare; one of them is chosen at load time.
func TestUnknownMemberLooksInSubclassesForThisAndStatic(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///sockets.php", `<?php
class ftp_sockets extends ftp_base {
    function _exec( $cmd ) {}
    static function make() {}
    const MODE = 1;
}
`)
	src := `<?php
class ftp_base {
    var $Verbose;
    function run( ftp_base $other ) {
        $this->_exec( 'TYPE I' );
        $this->_EXEC( 'QUIT' );
        static::make();
        $x = static::MODE;
        $this->verbose = true;
        $this->my_regexp( 'x' );
        $other->_exec( 'x' );
        self::make();
    }
}
`
	idx.IndexFile("file:///base.php", src)
	resolver := func(expr, source string, line int, file *parser.FileNode) string {
		if expr == "$other" || expr == "$this" {
			return "ftp_base"
		}
		return ""
	}
	rule := &UnknownMemberRule{TypeResolver: resolver}
	var got []string
	for _, f := range rule.Check(parser.ParseFile(src), src, idx) {
		got = append(got, strings.Split(f.Message, " on ")[0])
	}
	// verbose differs from the declared $Verbose (properties are case-sensitive),
	// my_regexp exists nowhere, and $other / self:: aren't late-bound.
	want := []string{
		"Unknown member 'verbose'",
		"Unknown member 'my_regexp'",
		"Unknown member '_exec'",
		"Unknown member 'make'",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
