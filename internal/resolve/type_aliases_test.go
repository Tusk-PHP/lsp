package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestRawMemberReturnTypeExpandsImportedAliases(t *testing.T) {
	idx := symbols.NewIndex()
	source := `<?php
namespace App;

class User {}
class Collection {}

/**
 * @phpstan-type Payload array{id: int, user: User}
 * @phpstan-type Users Collection<int, User>
 */
class Source {}

/**
 * @phpstan-import-type Payload from Source
 * @phpstan-import-type Users from Source as ImportedUsers
 */
class Consumer {
    /** @return ImportedUsers */
    public function users() {}

    /** @return Payload */
    public function payload() {}
}
`
	idx.IndexFile("file:///aliases.php", source)

	r := NewResolver(idx)
	file := parser.ParseFile(source)

	users := idx.Lookup("App\\Consumer::users")
	if users == nil {
		t.Fatal("expected App\\Consumer::users symbol")
	}
	if got := r.rawMemberReturnType(users); got != "App\\Collection<int, App\\User>" {
		t.Fatalf("unexpected expanded users return type %q", got)
	}
	usersRT := r.MemberTypeResolved(users, file, "", nil)
	if usersRT.FQN != "App\\Collection" {
		t.Fatalf("unexpected resolved users FQN %q", usersRT.FQN)
	}
	if len(usersRT.Params) != 2 || usersRT.Params[0].FQN != "int" || usersRT.Params[1].FQN != "App\\User" {
		t.Fatalf("unexpected resolved users params %+v", usersRT.Params)
	}

	payload := idx.Lookup("App\\Consumer::payload")
	if payload == nil {
		t.Fatal("expected App\\Consumer::payload symbol")
	}
	if got := r.rawMemberReturnType(payload); got != "array{id: int, user: App\\User}" {
		t.Fatalf("unexpected expanded payload return type %q", got)
	}
	if got := r.MemberType(payload, file); got != "array" {
		t.Fatalf("unexpected payload member type %q", got)
	}
}

func TestResolveVariableTypeTypedExpandsImportedAliasParams(t *testing.T) {
	idx := symbols.NewIndex()
	source := `<?php
namespace App;

class User {}

/**
 * @phpstan-type Payload array{id: int, user: User}
 */
class Source {}

/**
 * @phpstan-import-type Payload from Source
 */
class Consumer {
    /**
     * @param Payload $payload
     */
    public function handle($payload) {
        return $payload;
    }
}
`
	idx.IndexFile("file:///param_aliases.php", source)

	r := NewResolver(idx)
	file := parser.ParseFile(source)
	lines := SplitLines(source)

	rt := r.ResolveVariableTypeTyped("$payload", lines, protocolPos(18), file)
	if rt.FQN != "array" {
		t.Fatalf("unexpected payload param FQN %q", rt.FQN)
	}
	if rt.Shape != "array{id: int, user: App\\User}" {
		t.Fatalf("unexpected payload param shape %q", rt.Shape)
	}
}

func protocolPos(line int) protocol.Position {
	return protocol.Position{Line: line}
}
