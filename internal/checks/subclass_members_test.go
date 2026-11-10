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
	resolver := func(expr, source string, line, col int, file *parser.FileNode) string {
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

// An interface or abstract class can't be instantiated, so an object typed as
// one is always something below it, and calls to what those add are allowed:
// Magento's getResource() returns AbstractDb, whose EAV subclasses declare
// getAttribute(), and OrderItemInterface objects are Order\Item models.
func TestUnknownMemberLooksInImplementationsOfAbstractTypes(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///lib.php", `<?php
namespace Lib;
abstract class AbstractDb { public function load() {} }
abstract class AbstractEntity extends AbstractDb {}
class Eav extends AbstractEntity { public function getAttribute() {} }
interface ItemInterface { public function getSku(); }
interface OrderItemInterface extends ItemInterface {}
class Item implements OrderItemInterface { public function getSku() {} public function getQtyToShip() {} }
class Concrete { public function run() {} }
class Special extends Concrete { public function extra() {} }
`)
	src := `<?php
namespace App;
use Lib\AbstractDb;
use Lib\ItemInterface;
use Lib\Concrete;
function f(AbstractDb $db, ItemInterface $item, Concrete $c) {
    $db->getAttribute('sku');
    $db->missing();
    $item->getQtyToShip();
    $item->nowhere();
    $c->extra();
}
`
	idx.IndexFile("file:///app.php", src)
	resolver := func(expr, source string, line, col int, file *parser.FileNode) string {
		switch expr {
		case "$db":
			return "Lib\\AbstractDb"
		case "$item":
			return "Lib\\ItemInterface"
		case "$c":
			return "Lib\\Concrete"
		}
		return ""
	}
	rule := &UnknownMemberRule{TypeResolver: resolver}
	var got []string
	for _, f := range rule.Check(parser.ParseFile(src), src, idx) {
		got = append(got, f.Message)
	}
	// A concrete class may be the object's own class, so a subclass's method
	// is still reported on it.
	want := []string{
		"Unknown member 'missing' on 'AbstractDb'",
		"Unknown member 'nowhere' on 'ItemInterface'",
		"Unknown member 'extra' on 'Concrete'",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
