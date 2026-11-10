package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestUnknownSymbolsIgnoreCaseLikePHP(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///lib.php", `<?php
class PclZip { const OPT_PATH = 1; public function create() {} }
class stdClass {}
class ZipArchive { const CHECKCONS = 4; }
function floatHelper( $v ) { return $v; }
`)
	src := `<?php
function use_them( $p_archive ) {
    if ( $p_archive instanceof pclzip ) { $p_archive->create(); }
    $o = new StdClass();
    $z = ZIPARCHIVE::CHECKCONS;
    $p = PCLZIP::OPT_PATH;
    $f = FloatHelper( '1.5' );
    $m = new ZipArchiveMissing();
}
`
	file := parser.ParseFile(src)
	idx.IndexFile("file:///use.php", src)
	var got []string
	for _, rule := range []interface {
		Check(*parser.FileNode, string, *symbols.Index) []Finding
	}{&UnknownClassRule{}, &UnknownFunctionRule{}, &UnknownMemberRule{TypeResolver: func(string, string, int, int, *parser.FileNode) string { return "" }}} {
		for _, f := range rule.Check(file, src, idx) {
			got = append(got, f.Message)
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "ZipArchiveMissing") {
		t.Errorf("only the really missing class should be reported, got %q", got)
	}
}
