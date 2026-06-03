package symquery_test

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/symquery"
)

func setupIndex(sources map[string]string) (*symbols.Index, *analyzer.Analyzer) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for uri, src := range sources {
		idx.IndexFileWithSource(uri, src, symbols.SourceProject)
	}
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := analyzer.NewAnalyzer(idx, ca)
	return idx, a
}

func readSource(sources map[string]string) func(uri string) (string, bool) {
	return func(uri string) (string, bool) {
		src, ok := sources[uri]
		return src, ok
	}
}

func TestDeclarationByFQN_Found(t *testing.T) {
	sources := map[string]string{
		"file:///user.php": `<?php
namespace App\Models;
class User {}
`,
	}
	idx, _ := setupIndex(sources)

	sym, ok := symquery.DeclarationByFQN(idx, `App\Models\User`)
	if !ok {
		t.Fatal("expected symbol to be found")
	}
	if sym.FQN != `App\Models\User` {
		t.Errorf("unexpected FQN: %q", sym.FQN)
	}
}

func TestDeclarationByFQN_NotFound(t *testing.T) {
	idx, _ := setupIndex(map[string]string{})

	_, ok := symquery.DeclarationByFQN(idx, `No\Such\Class`)
	if ok {
		t.Fatal("expected not found")
	}
}

func TestReferencesByFQN_ReturnsLocations(t *testing.T) {
	sources := map[string]string{
		"file:///user.php": `<?php
namespace App\Models;
class User {}
`,
		"file:///controller.php": `<?php
namespace App\Http;
use App\Models\User;
class UserController {
    public function show(User $u): User { return $u; }
}
`,
	}
	idx, an := setupIndex(sources)

	locs, ok := symquery.ReferencesByFQN(idx, an, `App\Models\User`, readSource(sources))
	if !ok {
		t.Fatal("expected ReferencesByFQN to return ok")
	}
	if len(locs) == 0 {
		t.Fatal("expected at least one reference location")
	}
}

func TestReferencesByFQN_UnknownFQN(t *testing.T) {
	idx, an := setupIndex(map[string]string{})

	_, ok := symquery.ReferencesByFQN(idx, an, `No\Such\Class`, readSource(map[string]string{}))
	if ok {
		t.Fatal("expected not found for unknown FQN")
	}
}
