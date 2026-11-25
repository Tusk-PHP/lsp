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
