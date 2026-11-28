package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const inheritedDocReturnSource = `<?php
namespace App;

class Money {}
class Handler {}
class MoneyHandler extends Handler {}

/** @template T */
interface Reader
{
    /** @return T|null */
    public function read();
}

interface Factory
{
    /** @return class-string<Handler>|Handler */
    public static function make();
}

/** @implements Reader<Money> */
class DocumentedReader implements Reader
{
    public function read() {}
}

class TypedReader implements Reader
{
    public function read(): Money {}
}

/** @implements Reader<Money> */
class TypedDocumentedReader implements Reader
{
    public function read(): ?Money {}
}

class MoneyFactory implements Factory
{
    public static function make(): MoneyHandler {}
}

class UnionReader implements Reader
{
    public function read(): Money|Handler {}
}

class ListReader implements Reader
{
    public function read(): array {}
}
`

func TestInheritedPHPDocReturnAgainstTheNativeType(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app.php", inheritedDocReturnSource)
	r := NewResolver(idx)
	tests := []struct{ method, want string }{
		{`App\DocumentedReader::read`, `?App\Money`},
		{`App\TypedReader::read`, `App\Money`},
		{`App\TypedDocumentedReader::read`, `?App\Money`},
		{`App\MoneyFactory::make`, `App\MoneyHandler`},
		{`App\UnionReader::read`, `App\Money|App\Handler`},
		{`App\ListReader::read`, "array"},
	}
	for _, tt := range tests {
		member := idx.Lookup(tt.method)
		if member == nil {
			t.Fatalf("%s is not indexed", tt.method)
		}
		if got := r.MemberTypeForCall(member, nil, member.ParentFQN, nil, nil).FullString(); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.method, got, tt.want)
		}
		if got := r.MemberTypeForCall(member, nil, "", nil, nil).FullString(); got != tt.want {
			t.Errorf("%s without a caller: got %q, want %q, as from its own class", tt.method, got, tt.want)
		}
	}
}
