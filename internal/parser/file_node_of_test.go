package parser

import "testing"

func TestFileNodeOfReusesTheParseResult(t *testing.T) {
	file := FileNodeOf(New().Parse("<?php\nnamespace App;\nclass Shop {}\n"))
	if file == nil || file.Namespace != "App" || len(file.Classes) != 1 || file.Classes[0].Name != "Shop" {
		t.Fatalf("FileNodeOf = %+v", file)
	}
}

func TestFileNodeOfIsNilWhenBuildingPanics(t *testing.T) {
	if file := FileNodeOf(nil); file != nil {
		t.Errorf("FileNodeOf(nil) = %+v, want nil", file)
	}
}
