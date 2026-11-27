package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCalledFunctionFQNFallsBackToTheGlobalFunctionAsPHPDoes(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/helpers.php", "<?php\nnamespace App\\Support;\nfunction env($key) {}\n")
	imported := []parser.UseNode{{FullName: `App\Support\env`, Alias: "env", Kind: "function"}}
	cases := []struct {
		name, namespace string
		uses            []parser.UseNode
		want            string
	}{
		{"env", "", nil, "env"},
		{"env", `App\Http`, nil, "env"},
		{"env", `App\Support`, nil, `App\Support\env`},
		{"ENV", `App\Support`, nil, `App\Support\env`},
		{`\env`, `App\Support`, nil, "env"},
		{"env", `App\Http`, imported, `App\Support\env`},
		{`Support\env`, "App", nil, `App\Support\env`},
		{"", "App", nil, ""},
	}
	for _, tc := range cases {
		if got := CalledFunctionFQN(idx, tc.name, tc.namespace, tc.uses); got != tc.want {
			t.Errorf("CalledFunctionFQN(%q in %q) = %q, want %q", tc.name, tc.namespace, got, tc.want)
		}
	}
}
