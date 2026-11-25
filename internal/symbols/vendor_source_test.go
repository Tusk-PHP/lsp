package symbols

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVendorSourcesAreReadFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Gateway.php")
	source := "<?php\nnamespace Lib;\n/** Charges cards. */\nclass Gateway { public function charge(): int { return 1; } }\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := "file://" + path
	idx := NewIndex()
	idx.IndexFileWithSource(uri, source, SourceVendor)

	if sym := idx.Lookup(`Lib\Gateway`); sym == nil || sym.DocComment != "/** Charges cards. */" {
		t.Fatalf("the class and its docblock are indexed: %+v", sym)
	}
	edited := source + "// changed on disk\n"
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := idx.GetFileSource(uri); got != edited {
		t.Errorf("a vendor source is read from disk, not kept: %q", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := idx.GetFileSource(uri); got != "" {
		t.Errorf("a vendor file that is gone has no source: %q", got)
	}

	idx.IndexFileWithSource(uri, source, SourceProject)
	if idx.GetFileSource(uri) != source {
		t.Error("a project file keeps its source")
	}
	if idx.GetFileSource("file:///never/indexed.php") != "" {
		t.Error("an unknown file has no source")
	}
}

func TestReindexingUnchangedVendorTextKeepsWhatAnalysisAdded(t *testing.T) {
	idx := NewIndex()
	const uri = "file:///vendor/lib/Post.php"
	source := "<?php\nnamespace Lib;\nclass Post { public function comments() {} }\n"
	idx.IndexFileWithSource(uri, source, SourceVendor)
	idx.AddVirtualMember(`Lib\Post`, &Symbol{Name: "$comments", FQN: `Lib\Post::$comments`, Kind: KindProperty, URI: uri, IsVirtual: true})
	generation := idx.Generation()

	idx.IndexFileWithSource(uri, source, SourceVendor)
	if idx.Lookup(`Lib\Post::$comments`) == nil || idx.Generation() != generation {
		t.Error("re-indexing the same vendor text, which the index doesn't keep, changes nothing")
	}

	idx.IndexFileWithSource(uri, source+"\n", SourceVendor)
	if idx.Lookup(`Lib\Post::$comments`) != nil {
		t.Error("edited vendor text is indexed again")
	}

	idx.IndexFileWithSource(uri, source, SourceProject)
	if sym := idx.Lookup(`Lib\Post`); sym == nil || sym.Source != SourceProject || idx.GetFileSource(uri) != source {
		t.Errorf("the same text indexed as project code is indexed again and keeps its source: %+v", sym)
	}
	idx.IndexFileWithSource(uri, source, SourceVendor)
	if sym := idx.Lookup(`Lib\Post`); sym == nil || sym.Source != SourceVendor {
		t.Errorf("and back as vendor code: %+v", sym)
	}
}

func TestShadowedVendorDeclarationKeepsItsSourceOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Str.php")
	vendor := "<?php\nnamespace Lib;\n/** From the package. */\nclass Str { public function of(string $s): self { return $this; } }\n"
	if err := os.WriteFile(path, []byte(vendor), 0o644); err != nil {
		t.Fatal(err)
	}
	vendorURI := "file://" + path
	idx := NewIndex()
	idx.IndexFileWithSource(vendorURI, vendor, SourceVendor)
	idx.IndexFileWithSource("file:///app/Str.php", "<?php\nnamespace Lib;\nclass Str { public function local() {} }\n", SourceProject)

	defs := idx.Definitions(`Lib\Str`)
	if len(defs) != 2 {
		t.Fatalf("both declarations are kept: %d", len(defs))
	}
	shadowed := defs[1]
	if shadowed.URI != vendorURI || idx.GetFileSource(shadowed.URI) != vendor {
		t.Errorf("the shadowed vendor declaration's source is read from disk: %s", shadowed.URI)
	}
	members := idx.DefinitionMembers(shadowed)
	if len(members) != 1 || members[0].Name != "of" || members[0].Params[0].Name != "$s" {
		t.Errorf("the shadowed vendor declaration keeps its members: %+v", members)
	}

	idx.IndexFileWithSource("file:///app/Str.php", "<?php\n", SourceProject)
	if sym := idx.Lookup(`Lib\Str`); sym == nil || sym.URI != vendorURI || sym.DocComment != "/** From the package. */" {
		t.Fatalf("the vendor declaration comes back with its detached strings: %+v", sym)
	}
	if m := idx.Lookup(`Lib\Str::of`); m == nil || len(m.Params) != 1 || m.Params[0].Name != "$s" {
		t.Errorf("and its members: %+v", m)
	}
	if idx.GetFileSource(vendorURI) != vendor {
		t.Error("and its source is still read from disk")
	}
}
