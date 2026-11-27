package sourcectx

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

const anonymousRoom = `<?php
namespace App;
class Room {
    public function area() {}
    public function make() {
        $room = 1;
        return new class extends Base {
            public function area() {
                return 2;
            }
        };
    }
}
`

func TestCursorInsideAnAnonymousClassIsInThatClass(t *testing.T) {
	at := func(needle string) *CursorContext {
		i := strings.Index(anonymousRoom, needle)
		pos := protocol.Position{Line: strings.Count(anonymousRoom[:i], "\n"), Character: i - strings.LastIndex(anonymousRoom[:i], "\n") - 1}
		return analyzeText("file:///Room.php", anonymousRoom, pos, phpparser.OnlyGrammar(token.Latest))
	}
	inside := at("return 2")
	if !parser.IsAnonymousClass(inside.EnclosingFQN) {
		t.Errorf("enclosing class inside the anonymous class = %q, want the anonymous class", inside.EnclosingFQN)
	}
	if inside.Scope == nil || inside.Scope.MethodName != "area" || inside.Scope.StartLine != 7 {
		t.Errorf("scope inside the anonymous class's area() = %+v, want that method", inside.Scope)
	}
	outside := at("$room = 1")
	if outside.EnclosingFQN != `App\Room` || outside.Scope == nil || outside.Scope.MethodName != "make" {
		t.Errorf("around the anonymous class: class %q, scope %+v; want App\\Room and make()", outside.EnclosingFQN, outside.Scope)
	}
}

func TestCursorInANestedAnonymousClassBodyIsInNoMethod(t *testing.T) {
	src := `<?php
namespace App;
$outer = new class(OUTER_SEED) {
    public function make() {
        return new class(INNER_SEED) {
            public $x = 1;
            public function inner() {
                return 2;
            }
        };
    }
};
`
	at := func(needle string) *CursorContext {
		i := strings.Index(src, needle)
		pos := protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1}
		return analyzeText("file:///Nested.php", src, pos, phpparser.OnlyGrammar(token.Latest))
	}
	property := at("$x = 1")
	if !strings.Contains(property.EnclosingFQN, "@5@") || property.Scope != nil {
		t.Errorf("in the inner class's property: class %q, scope %+v; want the inner class and no scope", property.EnclosingFQN, property.Scope)
	}
	inner := at("return 2")
	if !strings.Contains(inner.EnclosingFQN, "@5@") || inner.Scope == nil || inner.Scope.MethodName != "inner" {
		t.Errorf("in inner(): class %q, scope %+v; want the inner class and inner()", inner.EnclosingFQN, inner.Scope)
	}
	innerArg := at("INNER_SEED")
	if !strings.Contains(innerArg.EnclosingFQN, "@3@") || innerArg.Scope == nil || innerArg.Scope.MethodName != "make" {
		t.Errorf("in the inner class's arguments: class %q, scope %+v; want the outer class and make()", innerArg.EnclosingFQN, innerArg.Scope)
	}
	outerArg := at("OUTER_SEED")
	if outerArg.EnclosingFQN != "" || outerArg.Scope != nil {
		t.Errorf("in the outer class's arguments: class %q, scope %+v; want no class and no scope", outerArg.EnclosingFQN, outerArg.Scope)
	}
}
