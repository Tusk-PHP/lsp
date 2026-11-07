package symbols

import (
	"strings"
	"testing"
)

// factories builds `XFactory` for any class X, like a framework generator.
type factories struct{ calls int }

func (f *factories) Candidate(fqn string) bool { return strings.HasSuffix(fqn, "Factory") }

func (f *factories) Generate(fqn string, idx *Index) (string, string, bool) {
	f.calls++
	source := strings.TrimSuffix(fqn, "Factory")
	if idx.Lookup(source) == nil {
		return "", "", false
	}
	ns, name := "", fqn
	if i := strings.LastIndex(fqn, `\`); i >= 0 {
		ns, name = fqn[:i], fqn[i+1:]
	}
	return "<?php\nnamespace " + ns + ";\nclass " + name + " {\n/** @return \\" + source + " */\npublic function create(array $data = []) {}\n}\n", source, true
}

func TestGeneratedClassesAreBuiltOnLookup(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///Product.php", "<?php\nnamespace App;\nclass Product {}\n")
	if idx.Lookup(`App\ProductFactory`) != nil {
		t.Fatal("no generator: nothing should be generated")
	}
	gen := &factories{}
	idx.SetClassGenerator(gen)

	sym := idx.Lookup(`App\ProductFactory`)
	if sym == nil || sym.Source != SourceGenerated {
		t.Fatalf("ProductFactory = %+v, want a generated class", sym)
	}
	if sym.URI != "file:///Product.php" {
		t.Errorf("generated class points at %q, want the class it's generated from", sym.URI)
	}
	if m := idx.Lookup(`App\ProductFactory::create`); m == nil {
		t.Error("create() missing")
	}
	if idx.Lookup(`App\MissingFactory`) != nil {
		t.Error("a factory of an unknown class must stay unknown")
	}
	for _, uri := range idx.GetAllFileURIs() {
		if strings.HasPrefix(uri, generatedScheme) {
			t.Errorf("GetAllFileURIs lists generated %q, which is not a file", uri)
		}
	}

	// Built once, then looked up like any class.
	calls := gen.calls
	idx.Lookup(`App\ProductFactory`)
	idx.GetClassMembers(`App\ProductFactory`)
	if gen.calls != calls {
		t.Errorf("generator called again for a class it already built")
	}

	// Removing the source class removes what was generated from it.
	idx.IndexFile("file:///Product.php", "<?php\nnamespace App;\n")
	if idx.Lookup(`App\ProductFactory`) != nil {
		t.Error("factory outlived the class it was generated from")
	}

	// A real declaration replaces the generated class.
	idx.IndexFile("file:///Product.php", "<?php\nnamespace App;\nclass Product {}\n")
	if idx.Lookup(`App\ProductFactory`) == nil {
		t.Fatal("factory not rebuilt after the class came back")
	}
	idx.IndexFile("file:///ProductFactory.php", "<?php\nnamespace App;\nclass ProductFactory { public function make() {} }\n")
	sym = idx.Lookup(`App\ProductFactory`)
	if sym == nil || sym.Source != SourceProject || idx.Lookup(`App\ProductFactory::create`) != nil {
		t.Errorf("real ProductFactory should replace the generated one, got %+v", sym)
	}
	idx.IndexFile("file:///Product.php", "<?php\nnamespace App;\nclass Product {}\n")
	if idx.Lookup(`App\ProductFactory::make`) == nil {
		t.Error("reindexing the source class removed the real ProductFactory")
	}
}
