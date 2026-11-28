package symbols

import "testing"

func TestSetPropertyTypeReplacesOnlyAPropertysType(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///order.php", "<?php\nnamespace App;\nclass Order {\n    public string $total;\n    public function total() {}\n}\n")
	idx.SetPropertyType(`App\Order::$total`, "?string")
	if got := idx.Lookup(`App\Order::$total`).Type; got != "?string" {
		t.Errorf("$total: %q, want ?string", got)
	}
	idx.SetPropertyType(`App\Order::total`, "int")
	if got := idx.Lookup(`App\Order::total`); got.Kind != KindMethod || got.Type != "" {
		t.Errorf("a method was given a property type: %+v", got)
	}
	idx.SetPropertyType(`App\Order::$missing`, "int")
	if idx.Lookup(`App\Order::$missing`) != nil {
		t.Error("setting the type of a missing property added one")
	}
}
