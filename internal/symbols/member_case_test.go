package symbols

import (
	"slices"
	"strings"
	"testing"
)

const memberCaseSource = `<?php
namespace App;
class Shape {
    public $Area;
    public $area;
    public function AREA() {}
    const area = 1;
    const Area = 2;
    public function computeTheEnclosedSurfaceOfThisShapeInSquareMetresPrecisely() {}
}
class Label {
    public function area() {}
}
`

func memberLabels(members []*Symbol) []string {
	names := make([]string, 0, len(members))
	for _, m := range members {
		names = append(names, m.ParentFQN+"::"+m.Name)
	}
	return names
}

func TestClassMembersNamedMatchesMethodsIgnoringCaseOtherMembersExactly(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///shape.php", memberCaseSource)
	for _, c := range []struct {
		class, name string
		want        []string
	}{
		{`App\Shape`, "area", []string{`App\Shape::$area`, `App\Shape::AREA`, `App\Shape::area`}},
		{`App\Shape`, "Area", []string{`App\Shape::$Area`, `App\Shape::AREA`, `App\Shape::Area`}},
		{`App\Shape`, "aREA", []string{`App\Shape::AREA`}},
		{`App\Shape|App\Label`, "area", []string{`App\Shape::$area`, `App\Shape::AREA`, `App\Shape::area`, `App\Label::area`}},
		{`App\Shape|App\Label`, "aREA", []string{`App\Shape::AREA`, `App\Label::area`}},
		{`App\Shape`, "COMPUTETHEENCLOSEDSURFACEOFTHISSHAPEINSQUAREMETRESPRECISELY", []string{`App\Shape::computeTheEnclosedSurfaceOfThisShapeInSquareMetresPrecisely`}},
	} {
		got := memberLabels(idx.ClassMembersNamed(c.class, c.name))
		slices.Sort(got)
		want := slices.Clone(c.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("ClassMembersNamed(%s, %s) = %v, want %v", c.class, c.name, got, want)
		}
	}
}

func TestClassMembersNamedKeepsTheHierarchyOrderWhenFiltering(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///shape.php", memberCaseSource)
	got := memberLabels(idx.ClassMembersNamed(`App\Shape`, "area"))
	want := []string{`App\Shape::$area`, `App\Shape::AREA`, `App\Shape::area`}
	if !slices.Equal(got, want) {
		t.Errorf("ClassMembersNamed(Shape, area) = %v, want %v in declaration order", got, want)
	}
}

func TestFoldKeyLowerCasesLongAndNonASCIINames(t *testing.T) {
	long := strings.Repeat("AbC", 30)
	var buf [64]byte
	for _, s := range []string{"MixedCase", long, "ÉtatSTATE"} {
		if got, want := string(FoldKey(&buf, s)), strings.ToLower(s); got != want {
			t.Errorf("FoldKey(%q) = %q, want %q", s, got, want)
		}
	}
}
