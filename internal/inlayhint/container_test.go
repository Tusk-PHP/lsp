package inlayhint

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var containerClasses = map[string]string{
	"file:///vendor/Repository.php": "<?php\nnamespace Illuminate\\Config;\nclass Repository { public function get(string $key): mixed { return null; } }\n",
	"file:///vendor/Request.php":    "<?php\nnamespace Illuminate\\Http;\nclass Request { public function user(): ?\\App\\User { return null; } }\n",
	"file:///app/User.php":          "<?php\nnamespace App;\nclass User {}\nclass Kernel {}\n",
}

const containerCalls = `<?php
namespace App;

function boot($app) {
    $config = app('config');
    $request = resolve('request');
    $repo = $app->make(\Illuminate\Contracts\Config\Repository::class);
    $kernel = $app->make(Kernel::class);
    $user = app('request')->user();
    $all = config();
    $name = config('app.name');
    $unknown = app('nothing');
}
`

func TestContainerCallsAreHinted(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for uri, src := range containerClasses {
		idx.IndexFile(uri, src)
	}
	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()
	p := NewProvider(idx, ca)
	hints := p.GetInlayHints("file:///app/boot.php", containerCalls, allCfg())
	for anchor, want := range map[string]string{
		"$config = ":  ": Repository",
		"$request = ": ": Request",
		"$repo = ":    ": Repository",
		"$kernel = ":  ": Kernel",
		"$user = ":    ": ?User",
		"$all = ":     ": Repository",
	} {
		line := lineOf(containerCalls, anchor)
		found := false
		for _, h := range hintsOnLine(hints, line) {
			found = found || h.Label == want
		}
		if !found {
			t.Errorf("%s: want %q, got %+v", anchor, want, hintsOnLine(hints, line))
		}
	}
	for _, anchor := range []string{"$name = ", "$unknown = "} {
		if got := hintsOnLine(hints, lineOf(containerCalls, anchor)); len(got) != 0 {
			t.Errorf("%s: want no hint, got %+v", anchor, got)
		}
	}
	if got := NewProvider(idx, nil).GetInlayHints("file:///app/boot.php", containerCalls, allCfg()); len(hintsOnLine(got, lineOf(containerCalls, "$config = "))) != 0 {
		t.Errorf("without a container: %+v", hintsOnLine(got, lineOf(containerCalls, "$config = ")))
	}
}
