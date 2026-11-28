package symbols

import "testing"

func TestTemplateFallbacks(t *testing.T) {
	idx := NewIndex()
	if _, ok := idx.TemplateFallbackOf(`App\Bag`); ok {
		t.Fatal("a fallback before any were set")
	}
	idx.SetTemplateFallbacks(map[string]TemplateFallback{`App\Bag`: {Params: []string{"T"}, Methods: map[string]string{"first": "?T"}}})
	f, ok := idx.TemplateFallbackOf(`App\Bag`)
	if !ok || f.Methods["first"] != "?T" || len(f.Params) != 1 {
		t.Errorf("TemplateFallbackOf = %+v, %v", f, ok)
	}
}
