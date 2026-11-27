package symbols

import (
	"slices"
	"testing"
)

const docMethodClassDoc = `/**
 * @method void configure(array{a: int, b: int} $opts, Closure(A, B): C $cb)
 * @method static Post fill(Collection $items, array $x = [1, 2], string $s = 'a,b', string &...$rest)
 */`

func TestDocMethodParamsSplitOnTopLevelCommas(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///post.php", "<?php\nnamespace App;\n\nuse Illuminate\\Support\\Collection;\n\n"+docMethodClassDoc+"\nclass Post {}\n")
	assertDocMethodParams(t, idx)
}

func TestIDEHelperDocMethodParamsSplitOnTopLevelCommas(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///post.php", "<?php\nnamespace App;\n\nclass Post {}\n")
	idx.IndexIDEHelperFile("file:///_ide_helper_models.php", "<?php\nnamespace App;\n\nuse Illuminate\\Support\\Collection;\n\n"+docMethodClassDoc+"\nclass Post {}\n")
	assertDocMethodParams(t, idx)
}

func assertDocMethodParams(t *testing.T, idx *Index) {
	t.Helper()
	configure := idx.Lookup(`App\Post::configure`)
	if configure == nil {
		t.Fatal("expected the virtual method configure")
	}
	var names []string
	for _, p := range configure.Params {
		names = append(names, p.Name)
	}
	if want := []string{"$opts", "$cb"}; !slices.Equal(names, want) {
		t.Errorf("configure param names = %q, want %q", names, want)
	}
	fill := idx.Lookup(`App\Post::fill`)
	if fill == nil {
		t.Fatal("expected the virtual method fill")
	}
	if !fill.IsStatic {
		t.Error("fill should be static")
	}
	want := []ParamInfo{
		{Name: "$items", Type: `Illuminate\Support\Collection`},
		{Name: "$x", Type: "array", DefaultValue: "[1, 2]"},
		{Name: "$s", Type: "string", DefaultValue: "'a,b'"},
		{Name: "$rest", Type: "string", IsVariadic: true, IsReference: true},
	}
	if !slices.Equal(fill.Params, want) {
		t.Errorf("fill params:\n got %+v\nwant %+v", fill.Params, want)
	}
}
