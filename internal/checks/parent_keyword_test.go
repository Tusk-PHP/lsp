package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestUnknownClassReportsMissingParentOnlyAtExtends(t *testing.T) {
	// Zend_Db_Select isn't indexed: report it where it's named, not at every
	// parent:: in the class. self and static are never unknown.
	src := `<?php
class Select extends Zend_Db_Select {
    public function __construct( $adapter ) {
        parent::__construct( $adapter );
        $a = new parent( $adapter );
        $b = new self( $adapter );
        $c = new static( $adapter );
        $d = self::class . static::class . PARENT::class;
        return $a instanceof self;
    }
}
`
	got := (&UnknownClassRule{}).Check(parser.ParseFile(src), src, symbols.NewIndex())
	if len(got) != 1 || got[0].StartLine != 1 || got[0].Message != "Unknown class 'Zend_Db_Select'" {
		t.Errorf("want only the extends line reported, got %+v", got)
	}
}
