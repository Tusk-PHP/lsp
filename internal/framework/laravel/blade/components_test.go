package blade

import (
	"strings"
	"testing"
)

func TestProps(t *testing.T) {
	src := "@props(['type' => 'info', 'message', 'alertType' => strtoupper('a, b')])\n<div>{{ $message }}</div>"
	var got []string
	for _, a := range Props(src) {
		got = append(got, a.Name+"="+a.Default)
	}
	if strings.Join(got, " ") != "type='info' message= alert-type=strtoupper('a, b')" {
		t.Fatalf("got %q", got)
	}
}

type fakeComponents map[string]Component

func (c fakeComponents) ComponentNames(prefix string) []string {
	var out []string
	for n := range c {
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}

func (c fakeComponents) Component(name string) (Component, bool) {
	comp, ok := c[name]
	return comp, ok
}

func (c fakeComponents) ComponentsKnown() bool { return true }

func componentFeatures() *Features {
	return &Features{Components: fakeComponents{
		"alert": {Name: "alert", ClassFQN: `App\View\Components\Alert`, URI: "file:///app/Alert.php", Line: 6, Col: 6,
			ViewURI:    "file:///p/resources/views/components/alert.blade.php",
			Attributes: []Attribute{{Name: "type", Type: "string", Default: "'info'"}, {Name: "message", Type: "string"}}},
	}}
}

func complete(t *testing.T, f *Features, withCursor string) ([]Completion, bool, string) {
	t.Helper()
	offset := strings.IndexByte(withCursor, '|')
	src := withCursor[:offset] + withCursor[offset+1:]
	items, ok := f.Complete("file:///p/page.blade.php", src, offset)
	return items, ok, src
}

func TestComponentCompletion(t *testing.T) {
	f := componentFeatures()
	items, ok, src := complete(t, f, "<div>\n<x-al|")
	if !ok || len(items) != 1 || items[0].Label != "alert" || src[items[0].Start:items[0].End] != "al" {
		t.Fatalf("name completion: %+v %v", items, ok)
	}

	items, ok, _ = complete(t, f, `<x-alert type="warning" |`)
	if !ok || len(items) != 1 || items[0].Label != "message" || items[0].Insert != `message="$1"` {
		t.Fatalf("attribute completion should leave out type, which is set: %+v %v", items, ok)
	}

	items, ok, src = complete(t, f, `<x-alert :mes|>`)
	if !ok || len(items) != 1 || items[0].Insert != `message="\$$1"` || src[items[0].Start:items[0].End] != "mes" {
		t.Fatalf("bound attribute completion: %+v %v", items, ok)
	}

	// Inside a value, and on other tags, attributes aren't completed.
	if items, _, _ := complete(t, f, `<x-alert type="w|"`); len(items) != 0 {
		t.Fatalf("no attributes inside a value: %+v", items)
	}
	if items, _, _ := complete(t, f, `<div |`); len(items) != 0 {
		t.Fatalf("no attributes on HTML tags: %+v", items)
	}
}

func TestComponentHoverAndDefinition(t *testing.T) {
	f := componentFeatures()
	src := `<x-alert type="info" />`
	h, ok := f.Hover(src, 5)
	if !ok || !strings.Contains(h.Markdown, `App\View\Components\Alert`) || !strings.Contains(h.Markdown, "`message`") || src[h.Span.Start:h.Span.End] != "alert" {
		t.Fatalf("hover: %+v %v", h, ok)
	}
	locs, ok := f.Definition("file:///p/page.blade.php", src, 5)
	if !ok || len(locs) != 2 || locs[0].URI != "file:///app/Alert.php" || locs[0].Line != 6 || locs[1].URI != "file:///p/resources/views/components/alert.blade.php" {
		t.Fatalf("definition: %+v %v", locs, ok)
	}
}

func TestUnknownComponent(t *testing.T) {
	f := componentFeatures()
	src := `<x-alert/><x-missing/><x-slot:title>t</x-slot><x-slot name="a"/><x-dynamic-component :component="$c"/><x-mail::button/><livewire:counter/><flux:button/>`
	var got []string
	for _, d := range f.Diagnose(src) {
		got = append(got, d.Code+": "+d.Message+" @ "+src[d.Span.Start:d.Span.End])
	}
	if strings.Join(got, "\n") != "unknown-component: Component `x-missing` not found @ missing" {
		t.Fatalf("got %q", got)
	}
}
