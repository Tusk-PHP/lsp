package parser

import "testing"

func TestStructParserSkipsConstantsAndKeepsWhatFollows(t *testing.T) {
	result := New().Parse(`<?php
const LIMIT = 10;
const BARE;
class Colors {
    const string RED = 'red', BLUE = Colors::{$name};
    const GREEN = ['a' => 1];
    public function after(): void {}
}
function helper() {}
`)
	if len(result.Classes) != 1 || len(result.Classes[0].Methods) != 1 || result.Classes[0].Methods[0].Name != "after" {
		t.Fatalf("classes = %+v", result.Classes)
	}
	if len(result.Functions) != 1 || result.Functions[0].Name != "helper" {
		t.Errorf("functions = %+v", result.Functions)
	}
}
