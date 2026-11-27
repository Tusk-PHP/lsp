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
