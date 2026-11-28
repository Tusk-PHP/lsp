package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func TestViewVarTypesKeepUnionPartsWhole(t *testing.T) {
	c := newVarCollector()
	c.add(ViewVar{Name: "items"}, "Collection<int, int|string>|null")
	c.add(ViewVar{Name: "items"}, "?App\\Order")
	c.add(ViewVar{Name: "row"}, "array{id: int|string}")
	got := map[string]string{}
	for _, v := range c.vars() {
		got[v.Name] = v.Type
	}
	want := map[string]string{
		"items": "App\\Order|Collection<int, int | string>|null",
		"row":   "array{id: int | string}",
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("$%s: %q, want %q", name, got[name], w)
		}
	}
}

func TestQualifyKeepsUnionPartsWhole(t *testing.T) {
	src := `<?php
namespace App\View;
use App\Models\Order;
use App\Support as Help;
`
	uri := "file:///app/View/Card.php"
	st := &typingState{files: map[string]siteFile{uri: {source: src, file: parser.ParseFile(src)}}}
	vt := &ViewTypes{}
	for typ, want := range map[string]string{
		"?Order":                        "App\\Models\\Order|null",
		"?Order|Item":                   "App\\Models\\Order|App\\View\\Item|null",
		"Order|null":                    "App\\Models\\Order|null",
		"(Order&\\Countable)|Item":      "(App\\Models\\Order&Countable)|App\\View\\Item",
		"array{a: int|string}|Order":    "array{a: int | string}|App\\Models\\Order",
		"\\Illuminate\\Support\\Fluent": "Illuminate\\Support\\Fluent",
		"int":                           "int",
		"Help\\Money|null":              "App\\Support\\Money|null",
		"Vendor\\Thing":                 "Vendor\\Thing",
	} {
		if got := vt.qualify(typ, uri, st); got != want {
			t.Errorf("qualify(%s) = %q, want %q", typ, got, want)
		}
	}
}
