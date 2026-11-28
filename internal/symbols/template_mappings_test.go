package symbols

import (
	"reflect"
	"testing"
)

func TestTemplateMappingOfReturnsWhatWasSet(t *testing.T) {
	idx := NewIndex()
	if _, ok := idx.TemplateMappingOf(`App\Box`); ok {
		t.Fatal("a fresh index maps App\\Box")
	}

	box := TemplateMapping{Params: []string{"TItem"}, Methods: map[string]string{"first": "?TItem"}}
	idx.SetTemplateMappings(map[string]TemplateMapping{`App\Box`: box})
	got, ok := idx.TemplateMappingOf(`App\Box`)
	if !ok || !reflect.DeepEqual(got, box) {
		t.Fatalf("TemplateMappingOf(App\\Box) = %v, %v; want %v, true", got, ok, box)
	}
	if _, ok := idx.TemplateMappingOf(`App\Crate`); ok {
		t.Error("App\\Crate is mapped although only App\\Box was set")
	}

	idx.SetTemplateMappings(map[string]TemplateMapping{`App\Crate`: box})
	if _, ok := idx.TemplateMappingOf(`App\Box`); ok {
		t.Error("App\\Box is still mapped after the mappings were replaced")
	}
	if _, ok := idx.TemplateMappingOf(`App\Crate`); !ok {
		t.Error("App\\Crate is not mapped after the mappings were replaced")
	}
}
