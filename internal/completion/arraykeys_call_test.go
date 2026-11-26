package completion

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var shapeLibrary = map[string]string{
	"file:///other.php":   "<?php\nnamespace Other;\n/** @return array{stray: int} */\nfunction row(): array {}\n",
	"file:///helpers.php": "<?php\nnamespace App\\Helpers;\n/** @return array{cents: int, currency: string} */\nfunction money(): array {}\n",
	"file:///repo.php": `<?php
namespace App;
class First {
    /** @return array{wrong: int} */
    public function load(): array {}
}
class Repo {
    /** @return array{id: int, title: string} */
    public function load(): array {}
}
`,
}

func arrayKeyLabels(t *testing.T, p *Provider, source, cursorLine string) []string {
	t.Helper()
	lines := strings.Split(source, "\n")
	for i, l := range lines {
		if strings.Contains(l, cursorLine) {
			var labels []string
			for _, item := range p.GetCompletions("file:///app/use.php", source, protocol.Position{Line: i, Character: len(l)}) {
				labels = append(labels, item.Label)
			}
			slices.Sort(labels)
			return labels
		}
	}
	t.Fatalf("line %q not found", cursorLine)
	return nil
}

func TestArrayKeysFromACallFollowPHPsNamesAndTheReceiversType(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///global.php", "<?php\n/** @return array{global: int} */\nfunction money(): array {}\n")
	for _, uri := range []string{"file:///other.php", "file:///helpers.php", "file:///repo.php"} {
		idx.IndexFile(uri, shapeLibrary[uri])
	}
	p := NewProvider(idx, nil, "")

	for _, tc := range []struct {
		name, source, cursor string
		want                 []string
	}{
		{"imported function", "<?php\nnamespace App\\Http;\nuse function App\\Helpers\\money;\n$m = money();\n$m['", "$m['", []string{"cents", "currency"}},
		{"function from another namespace", "<?php\nnamespace App\\Http;\n$r = row();\n$r['", "$r['", nil},
		{"method of $this's class", "<?php\nnamespace App;\nclass Before {}\nclass Repo {\n    public function show() {\n        $r = $this->load();\n        $r['", "$r['", []string{"id", "title"}},
		{"method of the receiver's class", "<?php\nnamespace App;\nfunction f(Repo $repo) {\n    $r = $repo->load();\n    $r['", "$r['", []string{"id", "title"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := arrayKeyLabels(t, p, tc.source, tc.cursor); !slices.Equal(got, tc.want) {
				t.Errorf("keys = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestArrayKeysFromAFrameworkMethodUseTheReceiversType(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Models.php", `<?php
namespace App\Models;
class Post { public string $slug; }
class User { public string $email; public int $age; }
`)
	p := NewProvider(idx, nil, "laravel")
	p.SetArrayResolver(models.NewFrameworkArrayResolver(idx, "/project", "laravel"))
	source := "<?php\nnamespace App;\nuse App\\Models\\User;\nfunction show(User $user) {\n    $data = $user->toArray();\n    $data['"
	if got := arrayKeyLabels(t, p, source, "$data['"); !slices.Equal(got, []string{"age", "email"}) {
		t.Errorf("toArray() on a User gives the User's properties, got %v", got)
	}
}

func TestArrayKeysFromAFunctionWithoutAShapeWithAFrameworkResolver(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/helpers.php", "<?php\nnamespace App;\nfunction rows(): array {}\n")
	p := NewProvider(idx, nil, "laravel")
	p.SetArrayResolver(models.NewFrameworkArrayResolver(idx, "/project", "laravel"))
	source := "<?php\nnamespace App;\nfunction show() {\n    $data = rows();\n    $data['"
	if got := arrayKeyLabels(t, p, source, "$data['"); len(got) != 0 {
		t.Errorf("a function returning a plain array has no keys to offer, got %v", got)
	}
}
