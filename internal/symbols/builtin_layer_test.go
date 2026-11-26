package symbols

import (
	"fmt"
	"slices"
	"testing"
)

func builtinShape(idx *Index) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []string
	for fqn, sym := range idx.symbols {
		if sym.Source != SourceBuiltin {
			continue
		}
		out = append(out, fmt.Sprintf("%s %d %s %d %s %v %v", fqn, sym.Kind, sym.ReturnType, len(sym.Children), sym.Extends, sym.Implements, idx.inheritanceMap[fqn]))
	}
	slices.Sort(out)
	return out
}

func TestReplaceBuiltinsMatchesAFreshIndexOfTheNewProfile(t *testing.T) {
	from := BuiltinProfile{PHPVersion: "8.3", Extensions: []string{"redis", "mbstring"}}
	to := BuiltinProfile{PHPVersion: "7.4", Extensions: []string{"imagick"}}

	fresh := NewIndex()
	fresh.RegisterBuiltinsForProfile(to)

	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(from)
	if idx.Lookup("str_contains") == nil || idx.Lookup("Redis") == nil {
		t.Fatal("the 8.3 profile with redis has str_contains and Redis")
	}
	if err := idx.ReplaceBuiltins(to); err != nil {
		t.Fatal(err)
	}
	got, want := builtinShape(idx), builtinShape(fresh)
	if !slices.Equal(got, want) {
		t.Fatalf("after the swap the index holds %d builtins, a fresh 7.4 index %d", len(got), len(want))
	}
	if idx.Lookup("str_contains") != nil || idx.Lookup("Redis") != nil || idx.Lookup("Imagick::readImage") == nil {
		t.Error("PHP 8.0 functions and dropped extensions leave, an added extension comes in")
	}

	if err := idx.ReplaceBuiltins(from); err != nil {
		t.Fatal(err)
	}
	back := NewIndex()
	back.RegisterBuiltinsForProfile(from)
	if !slices.Equal(builtinShape(idx), builtinShape(back)) {
		t.Error("swapping back gives the builtins of the first profile again")
	}
}

func TestReplaceBuiltinsKeepsDeclarationsThatHideABuiltin(t *testing.T) {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "8.3"})
	polyfill := "file:///vendor/symfony/polyfill-php80/bootstrap.php"
	idx.IndexFileWithSource(polyfill, "<?php\nfunction str_contains(string $haystack, string $needle): bool { return true; }\nclass Stringable { public function polyfilled(): void {} }\n", SourceVendor)
	app := "file:///app/ArrayObject.php"
	idx.IndexFile(app, "<?php\nclass ArrayObject { public function mine(): void {} }\n")

	for _, profile := range []BuiltinProfile{{PHPVersion: "7.4"}, {PHPVersion: "8.2"}} {
		if err := idx.ReplaceBuiltins(profile); err != nil {
			t.Fatal(err)
		}
		if sym := idx.Lookup("str_contains"); sym == nil || sym.URI != polyfill {
			t.Errorf("PHP %s: the polyfill still declares str_contains: %+v", profile.PHPVersion, sym)
		}
		if idx.Lookup("Stringable::polyfilled") == nil || idx.Lookup("Stringable::__toString") != nil {
			t.Errorf("PHP %s: the polyfilled class keeps its own members only", profile.PHPVersion)
		}
		if sym := idx.Lookup("ArrayObject"); sym == nil || sym.URI != app || idx.Lookup("ArrayObject::getIterator") != nil {
			t.Errorf("PHP %s: the project's ArrayObject hides the builtin one: %+v", profile.PHPVersion, sym)
		}
		defs := idx.Definitions("ArrayObject")
		hidden := defs[len(defs)-1]
		if hidden.Source != SourceBuiltin {
			t.Fatalf("PHP %s: the builtin ArrayObject is kept under the project's: %+v", profile.PHPVersion, defs)
		}
		for _, member := range hidden.Children {
			if member.Name == "mine" {
				t.Errorf("PHP %s: the builtin ArrayObject takes no member from the project's class", profile.PHPVersion)
			}
		}
	}

	idx.RemoveFile(polyfill)
	idx.RemoveFile(app)
	if sym := idx.Lookup("str_contains"); sym == nil || sym.Source != SourceBuiltin {
		t.Errorf("with the polyfill gone, PHP 8.2's own str_contains is found: %+v", sym)
	}
	if idx.Lookup("Stringable::__toString") == nil || idx.Lookup("ArrayObject::getIterator") == nil {
		t.Error("the builtin classes come back with their members")
	}
	if idx.Lookup("ArrayObject::mine") != nil || idx.Lookup("Stringable::polyfilled") != nil {
		t.Error("the removed files' members leave")
	}

}

func TestRemoveFileForgetsTheFile(t *testing.T) {
	idx := NewIndex()
	uri := "file:///app/Gone.php"
	idx.IndexFile(uri, "<?php\nnamespace App;\nclass Gone { public function run(): void {} }\nfunction helper() {}\n")
	view := "file:///resources/views/gone.blade.php"
	idx.AddTemplateFile(view)
	if !idx.HasFile(uri) || !idx.HasFile(view) {
		t.Fatal("the index lists the PHP file and the template it was given")
	}

	idx.RemoveFile(uri)
	idx.RemoveFile(view)
	if idx.Lookup(`App\Gone`) != nil || idx.Lookup(`App\Gone::run`) != nil || idx.Lookup(`App\helper`) != nil {
		t.Error("the file's declarations leave the index")
	}
	if idx.HasFile(uri) || idx.HasFile(view) || len(idx.TemplateFileURIs()) != 0 {
		t.Error("the index no longer lists the files")
	}
}

func BenchmarkReplaceBuiltins(b *testing.B) {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "8.3"})
	profiles := []BuiltinProfile{{PHPVersion: "7.4"}, {PHPVersion: "8.3"}}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if err := idx.ReplaceBuiltins(profiles[i%2]); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

func TestFilesUnderListsTheIndexedFilesOfADirectory(t *testing.T) {
	idx := NewIndex()
	for _, uri := range []string{"file:///p/app/A.php", "file:///p/app/sub/B.php", "file:///p/app/sub/deep/C.php", "file:///p/application/D.php"} {
		idx.IndexFile(uri, "<?php\n")
	}
	idx.AddTemplateFile("file:///p/app/sub/view.blade.php")
	got := idx.FilesUnder("/p/app/sub")
	slices.Sort(got)
	want := []string{"file:///p/app/sub/B.php", "file:///p/app/sub/deep/C.php", "file:///p/app/sub/view.blade.php"}
	if !slices.Equal(got, want) {
		t.Errorf("files under /p/app/sub: got %v, want %v", got, want)
	}
	idx.RemoveFile("file:///p/app/sub/B.php")
	idx.RemoveFile("file:///p/app/sub/deep/C.php")
	idx.RemoveFile("file:///p/app/sub/view.blade.php")
	if got := idx.FilesUnder("/p/app/sub"); len(got) != 0 {
		t.Errorf("removed files leave the directory: %v", got)
	}
	idx.IndexFile("file:///p/app/A.php", "<?php\nclass A {}\n")
	if got := idx.FilesUnder("/p"); len(got) != 2 {
		t.Errorf("re-indexing keeps one entry per file, and a sibling with the same prefix is its own directory: %v", got)
	}
}

func TestFilesUnderLeavesOutDocumentsThatAreNotFiles(t *testing.T) {
	idx := NewIndex()
	for _, uri := range []string{"file:///p/A.php", "untitled:Scratch.php", "file:///p/a%zz.php"} {
		idx.IndexFile(uri, "<?php\n")
	}
	if got := idx.FilesUnder("/"); !slices.Equal(got, []string{"file:///p/A.php"}) {
		t.Errorf("only file URIs sit in a directory: %v", got)
	}
	if got := idx.FilesUnder("."); len(got) != 0 {
		t.Errorf("a document that is not a file has no directory: %v", got)
	}
	idx.RemoveFile("untitled:Scratch.php")
	idx.RemoveFile("file:///p/a%zz.php")
	if got := idx.GetAllFileURIs(); !slices.Equal(got, []string{"file:///p/A.php"}) {
		t.Errorf("documents that are not files leave the index: %v", got)
	}
}

func TestFileTreeListsWindowsWorkspaceFilesUnderADirectory(t *testing.T) {
	tree := fileTree{windows: true}
	for _, uri := range []string{`file://C:\proj\app\Billing\Invoice.php`, `file://C:\proj\app\Billing\Tax\Rate.php`, `file://C:\proj\app\BillingKept\Receipt.php`, "file:///C:/proj/app/Billing/Open.php"} {
		tree.add(uri)
	}
	got := tree.under(`C:\proj\app\Billing`)
	slices.Sort(got)
	want := []string{"file:///C:/proj/app/Billing/Open.php", `file://C:\proj\app\Billing\Invoice.php`, `file://C:\proj\app\Billing\Tax\Rate.php`}
	if !slices.Equal(got, want) {
		t.Errorf("files under C:\\proj\\app\\Billing: got %v, want %v", got, want)
	}
	for _, uri := range want {
		tree.remove(uri)
	}
	if got := tree.under(`C:\proj\app\Billing`); len(got) != 0 {
		t.Errorf("removed files leave the directory: %v", got)
	}
	if got := tree.under(`C:\proj`); len(got) != 1 {
		t.Errorf("the sibling directory stays: %v", got)
	}
}

func TestBuiltinProfilesAreEqualWhateverTheirExtensionsSpelling(t *testing.T) {
	a := BuiltinProfile{PHPVersion: "8.3", Extensions: []string{"Redis", "ext-intl", "redis"}}
	if b := (BuiltinProfile{PHPVersion: "8.3", Extensions: []string{"intl", "redis"}}); !a.Equal(b) {
		t.Errorf("%+v and %+v name the same builtins", a, b)
	}
	for _, b := range []BuiltinProfile{{PHPVersion: "8.2", Extensions: []string{"intl", "redis"}}, {PHPVersion: "8.3", Extensions: []string{"intl"}}} {
		if a.Equal(b) {
			t.Errorf("%+v and %+v differ", a, b)
		}
	}
}

func TestBuiltinsRegisteredAfterTheProjectStayUnderItsDeclarations(t *testing.T) {
	idx := NewIndex()
	app := "file:///app/ArrayObject.php"
	idx.IndexFile(app, "<?php\nclass ArrayObject { public function mine(): void {} }\n")
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "8.3"})

	if sym := idx.Lookup("ArrayObject"); sym == nil || sym.URI != app {
		t.Errorf("the project's ArrayObject is found, not the builtin registered after it: %+v", sym)
	}
	idx.RemoveFile(app)
	if sym := idx.Lookup("ArrayObject"); sym == nil || sym.Source != SourceBuiltin {
		t.Errorf("with the project's class gone, the builtin ArrayObject is found: %+v", sym)
	}
}
