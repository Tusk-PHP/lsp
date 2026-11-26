package lsp

import (
	"reflect"
	"testing"
)

type uriHolder struct {
	URI string `json:"uri"`
}

func TestCanonicalizeURIsLeavesUnexportedFieldsAlone(t *testing.T) {
	type params struct {
		URI    string `json:"uri"`
		cached uriHolder
	}
	p := params{URI: "file:///srv/team%40shop/a.php", cached: uriHolder{URI: "file:///srv/team%40shop/b.php"}}
	got := canonicalizeURIs(reflect.ValueOf(&p), nil)
	if p.URI != "file:///srv/team@shop/a.php" || p.cached.URI != "file:///srv/team%40shop/b.php" {
		t.Errorf("only the exported uri field is respelled: %+v", p)
	}
	if want := []respelledURI{{client: "file:///srv/team%40shop/a.php", canonical: "file:///srv/team@shop/a.php"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
