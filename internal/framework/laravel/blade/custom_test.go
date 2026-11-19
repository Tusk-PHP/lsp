package blade

import (
	"strings"
	"testing"
)

func testCustom() *Custom {
	return &Custom{
		Directives:   map[string]bool{"money": true, "capture": true, "endcapture": true},
		Conditionals: map[string]bool{"admin": true},
		Where: map[string]Location{
			"money": {URI: "file:///p/app/Providers/AppServiceProvider.php", Line: 12, Col: 8},
			"admin": {URI: "file:///p/app/Providers/AppServiceProvider.php", Line: 14, Col: 8},
		},
	}
}

func TestCustomDirectivesStructure(t *testing.T) {
	c := testCustom()
	src := "@admin x @elseadmin y @endadmin @unlessadmin z @endadmin @capture($a) b @endcapture @money($price)"
	if probs := CheckStructureCustom(src, ParseCustom(src, c), c); len(probs) != 0 {
		t.Fatalf("balanced custom blocks reported: %+v", probs)
	}
	src = "@admin x\n@capture($a) y"
	var got []string
	for _, p := range CheckStructureCustom(src, ParseCustom(src, c), c) {
		got = append(got, p.Message)
	}
	want := "`@admin` is never closed with `@endadmin`\n`@capture` is never closed with `@endcapture`"
	if strings.Join(got, "\n") != want {
		t.Fatalf("got %q", got)
	}
	// Without the registrations, @admin is text and @else outside a block is
	// left alone, as before.
	src = "@admin a @else b @endadmin"
	if probs := CheckStructure(src, Parse(src)); len(probs) != 0 {
		t.Fatalf("unknown custom conditional reported: %+v", probs)
	}
}

func TestCustomDirectivesCompile(t *testing.T) {
	c := testCustom()
	doc := CompileWith("@admin($user)\n{{ $a }}\n@elseadmin\n{{ $b }}\n@endadmin\n@money($price)", CompileOptions{Custom: c})
	if !doc.Structured || !strings.Contains(doc.PHP, "if ([$user]):") || !strings.Contains(doc.PHP, "endif;") {
		t.Fatalf("a custom conditional compiles to an if block:\n%s", doc.PHP)
	}
	if strings.Contains(doc.PHP, "$price") {
		t.Fatalf("a custom directive's arguments aren't read as PHP:\n%s", doc.PHP)
	}
}

func TestCustomDirectivesFeatures(t *testing.T) {
	c := testCustom()
	f := &Features{Custom: func() *Custom { return c }}
	src := "<p>@mon"
	items, ok := f.Complete("file:///p/v.blade.php", src, len(src))
	if !ok || len(items) != 1 || items[0].Label != "@money" || !strings.Contains(items[0].Doc, "AppServiceProvider.php:13") {
		t.Fatalf("custom directive completion: %+v", items)
	}
	src = "<p>@adm"
	items, _ = f.Complete("file:///p/v.blade.php", src, len(src))
	if len(items) != 1 || !strings.HasSuffix(items[0].Insert, "@endadmin") {
		t.Fatalf("custom conditional completion: %+v", items)
	}
	src = "@money($price) @endadmin"
	if h, ok := f.Hover(src, 2); !ok || !strings.Contains(h.Markdown, "Blade::directive") {
		t.Fatalf("custom directive hover: %+v", h)
	}
	if locs, ok := f.Definition("file:///p/v.blade.php", src, 2); !ok || len(locs) != 1 || locs[0].Line != 12 {
		t.Fatalf("custom directive definition: %+v", locs)
	}
	if locs, ok := f.Definition("file:///p/v.blade.php", "@admin x @endadmin", 12); !ok || len(locs) != 1 || locs[0].Line != 14 {
		t.Fatalf("@endadmin goes to Blade::if('admin'): %+v", locs)
	}
}
