package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverTypesContainerCalls(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///vendor/Request.php", "<?php\nnamespace Illuminate\\Http;\nclass Request {\n    /** The user making the request. */\n    public function user(): ?object { return null; }\n}\n")
	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()
	p := NewProvider(idx, ca, "laravel")
	source := "<?php\nfunction f() {\n    $request = app('request');\n    $request->user();\n    resolve('request')\n        ->user();\n}\n"
	for _, at := range []struct{ target, line string }{{"user", "$request->user"}, {"user", "->user();"}} {
		h := p.GetHover("file:///app/f.php", source, charPosOf(t, source, at.target, at.line))
		if h == nil || !strings.Contains(h.Contents.Value, "The user making the request.") {
			t.Errorf("%s: got %+v", at.line, h)
		}
	}
	if h := p.GetHover("file:///app/f.php", source, charPosOf(t, source, "$request", "$request->user")); h == nil || !strings.Contains(h.Contents.Value, "Request") {
		t.Errorf("$request: got %+v", h)
	}
}
