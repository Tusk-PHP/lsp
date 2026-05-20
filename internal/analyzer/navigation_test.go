package analyzer

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestTypeDefinitionVariable(t *testing.T) {
	a, src := setupAnalyzer(t)

	pos := charPosOf(t, src, "$handler", "$handler->getLogger")
	locs := a.FindTypeDefinition("file:///project/src/Service.php", src, pos)
	if len(locs) != 1 {
		t.Fatalf("expected 1 type definition, got %d", len(locs))
	}
	if !strings.Contains(locs[0].URI, "StreamHandler.php") {
		t.Fatalf("expected StreamHandler type definition, got %s", locs[0].URI)
	}
}

func TestTypeDefinitionMethodReturn(t *testing.T) {
	a, src := setupAnalyzer(t)

	pos := charPosOf(t, src, "getLogger", "getLogger()->info")
	locs := a.FindTypeDefinition("file:///project/src/Service.php", src, pos)
	if len(locs) != 1 {
		t.Fatalf("expected 1 type definition, got %d", len(locs))
	}
	if !strings.Contains(locs[0].URI, "Logger.php") {
		t.Fatalf("expected Logger type definition, got %s", locs[0].URI)
	}
}

func TestImplementationInterfaceAndMethod(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///contracts.php", `<?php
namespace App\Contracts;
interface CacheStore {
    public function get(string $key): string;
}
`)
	idx.IndexFile("file:///stores.php", `<?php
namespace App\Stores;
use App\Contracts\CacheStore;

abstract class BaseStore implements CacheStore {}

class RedisStore extends BaseStore {
    public function get(string $key): string { return $key; }
}

class FileStore implements CacheStore {
    public function get(string $key): string { return $key; }
}
`)

	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))
	source := `<?php
namespace App\Contracts;
interface CacheStore {
    public function get(string $key): string;
}
`

	typePos := protocol.Position{Line: 2, Character: strings.Index(strings.Split(source, "\n")[2], "CacheStore")}
	typeLocs := a.FindImplementation("file:///contracts.php", source, typePos)
	if len(typeLocs) != 2 {
		t.Fatalf("expected 2 concrete implementors, got %d", len(typeLocs))
	}

	methodPos := protocol.Position{Line: 3, Character: strings.Index(strings.Split(source, "\n")[3], "get")}
	methodLocs := a.FindImplementation("file:///contracts.php", source, methodPos)
	if len(methodLocs) != 2 {
		t.Fatalf("expected 2 concrete method implementations, got %d", len(methodLocs))
	}
}

func TestImplementationAbstractClass(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///handlers.php", `<?php
namespace App;

abstract class BaseHandler {
    abstract public function handle(): string;
}

class HttpHandler extends BaseHandler {
    public function handle(): string { return 'http'; }
}

abstract class MidHandler extends BaseHandler {}

class QueueHandler extends MidHandler {
    public function handle(): string { return 'queue'; }
}
`)

	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))
	source := idx.GetFileSource("file:///handlers.php")

	typePos := protocol.Position{Line: 3, Character: strings.Index(strings.Split(source, "\n")[3], "BaseHandler")}
	typeLocs := a.FindImplementation("file:///handlers.php", source, typePos)
	if len(typeLocs) != 2 {
		t.Fatalf("expected 2 concrete descendants, got %d", len(typeLocs))
	}

	methodPos := protocol.Position{Line: 4, Character: strings.Index(strings.Split(source, "\n")[4], "handle")}
	methodLocs := a.FindImplementation("file:///handlers.php", source, methodPos)
	if len(methodLocs) != 2 {
		t.Fatalf("expected 2 concrete method implementations, got %d", len(methodLocs))
	}
}
