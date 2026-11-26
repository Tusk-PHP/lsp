package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestVariableInAnUnterminatedStringKeepsItsOwnFunction(t *testing.T) {
	src := "<?php\nfunction other() {\n    /** @var array{id: int} $post */\n    $post = load();\n}\n\nfunction test($post) {\n    $post['"
	r, file, lines := setupBoxResolver(t, src)
	rt := r.ResolveVariableTypeTyped("$post", lines, protocol.Position{Line: 7, Character: 11}, file)
	if rt.FQN != "" || rt.Shape != "" {
		t.Fatalf("$post in test() = %+v, want no type: other()'s @var does not reach it", rt)
	}
}
