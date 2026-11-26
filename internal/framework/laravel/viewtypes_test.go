package laravel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestAnonymousComponentPropsAreTypedFromTheirView(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my app")
	writeFiles(t, root, map[string]string{
		"resources/views/components/alert.blade.php": "@props(['type' => 'info'])",
	})
	uri := symbols.PathToURI(filepath.Join(root, "resources", "views", "components", "alert.blade.php"))
	types := &ViewTypes{
		Data: NewViewData(root),
		Read: func(uri string) string {
			data, _ := os.ReadFile(symbols.URIToPath(uri))
			return string(data)
		},
	}

	in := types.For(uri)
	if len(in.Vars) != 1 {
		t.Fatalf("the component receives its one prop, got %+v", in.Vars)
	}
	v := in.Vars[0]
	if v.Name != "type" || v.Type != "string" {
		t.Errorf("prop = %s %s, want string type", v.Type, v.Name)
	}
	if len(v.Sites) != 1 || v.Sites[0].URI != uri {
		t.Errorf("the prop is declared in the component's view %q, got %+v", uri, v.Sites)
	}
}
