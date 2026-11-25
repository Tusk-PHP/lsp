package symbols

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestGlobalConstantSurvivesRemovalOfOneOfTwoDeclaringFiles(t *testing.T) {
	cases := []struct {
		name, fqn string
		src       [2]string
	}{
		{"global", "X", [2]string{"<?php\nconst X = 1;\n", "<?php\nconst X = 2;\n"}},
		{"namespaced", "App\\X", [2]string{"<?php\nnamespace App;\nconst X = 1;\n", "<?php\nnamespace App;\nconst X = 2;\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fqn, src := tc.fqn, tc.src
			idx := NewIndex()
			idx.IndexFile("file:///a.php", src[0])
			idx.IndexFile("file:///b.php", src[1])
			if got := definitionURIs(idx.Definitions(fqn)); !slices.Equal(got, []string{"file:///a.php", "file:///b.php"}) {
				t.Fatalf("both declarations of X are definitions: %v", got)
			}

			idx.IndexFile("file:///b.php", "<?php\n")
			if sym := idx.Lookup(fqn); sym == nil || sym.URI != "file:///a.php" || sym.Kind != KindConstant {
				t.Fatalf("removing the current declaration brings back the other one: %+v", sym)
			}
			if got := idx.LookupByName("X"); len(got) != 1 {
				t.Fatalf("X is still found by name: %v", got)
			}

			idx.IndexFile("file:///b.php", src[1])
			idx.IndexFile("file:///a.php", "<?php\n")
			if sym := idx.Lookup(fqn); sym == nil || sym.URI != "file:///b.php" {
				t.Fatalf("removing a replaced declaration keeps the current one: %+v", sym)
			}

			idx.IndexFile("file:///b.php", "<?php\n")
			if idx.Lookup(fqn) != nil || idx.Definitions(fqn) != nil {
				t.Fatal("X is gone once no file declares it")
			}
		})
	}
}

func BenchmarkReindexFileOfGlobalConstants(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\nnamespace App;\n")
	for i := range 500 {
		fmt.Fprintf(&src, "const C%d = %d;\n", i, i)
	}
	texts := [2]string{src.String(), src.String() + "\n"}
	idx := NewIndex()
	idx.IndexFile("file:///other.php", texts[0])
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		idx.IndexFile("file:///consts.php", texts[i%2])
		i++
	}
}
