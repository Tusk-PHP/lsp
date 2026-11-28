package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestProviderResolverTypesWithTheProvidersIndex(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nnamespace App;\nclass User {}\n")
	if got := NewProvider(idx, nil, "").Resolver().ResolveClassName(`App\User`, nil); got != `App\User` {
		t.Errorf("ResolveClassName = %q, want App\\User", got)
	}
	if NewProvider(idx, nil, "").Resolver().Index != idx {
		t.Error("the provider's resolver reads another index")
	}
}
