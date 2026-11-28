package symbols

import (
	"slices"
	"strconv"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

const enumSource = "<?php\nenum Suit { case Hearts; }\n"

func php80Index() *Index {
	idx := NewIndexFor(phpparser.OnlyGrammar(token.PHP80))
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: "8.0", Grammars: phpparser.OnlyGrammar(token.PHP80)})
	return idx
}

func moveTo(t *testing.T, idx *Index, grammars phpparser.GrammarRange) {
	t.Helper()
	if err := idx.ReplaceBuiltins(BuiltinProfile{PHPVersion: "8.1", Grammars: grammars}); err != nil {
		t.Fatal(err)
	}
}

func TestReparseForGrammarsStopsWhenNewerGrammarsArrive(t *testing.T) {
	idx := php80Index()
	for i := range reparseBatch + 1 {
		idx.IndexFile("file:///app/e"+strconv.Itoa(1000+i)+".php", "<?php\nenum E"+strconv.Itoa(i)+" { case A; }\n")
	}
	moveTo(t, idx, phpparser.OnlyGrammar(token.PHP81))
	calls := 0
	current := idx.ReparseForGrammars(func(string, string) {
		if calls == 0 {
			moveTo(t, idx, phpparser.OnlyGrammar(token.PHP82))
		}
		calls++
	})
	if current || calls != reparseBatch {
		t.Fatalf("current = %v after %d files, want the first batch only and false", current, calls)
	}
	if idx.Lookup("E"+strconv.Itoa(reparseBatch)) != nil {
		t.Error("the file after the first batch is left for the newer grammars")
	}
	if !idx.ReparseForGrammars(func(string, string) {}) || idx.Lookup("E"+strconv.Itoa(reparseBatch)) == nil {
		t.Error("the run for the newer grammars reads it")
	}
}

func TestReparseForGrammarsLeavesFilesIndexedMeanwhile(t *testing.T) {
	idx := php80Index()
	uri := "file:///app/Suit.php"
	idx.IndexFile(uri, enumSource)
	moveTo(t, idx, phpparser.OnlyGrammar(token.PHP81))
	epoch, grammars, candidates := idx.reparseCandidates()
	if len(candidates) != 1 {
		t.Fatalf("candidates = %+v, want Suit.php alone", candidates)
	}
	batch := reparse(candidates, grammars)
	idx.IndexFile(uri, "<?php\nclass Edited {}\n")
	swapped, current := idx.swapReparsed(epoch, batch)
	if !current || len(swapped) != 0 {
		t.Fatalf("swapped %+v (current %v), want nothing", swapped, current)
	}
	if idx.Lookup("Edited") == nil || idx.Lookup("Suit") != nil {
		t.Error("the newer text stays indexed")
	}
}

func TestReparseForGrammarsSkipsFilesWhoseGrammarStays(t *testing.T) {
	idx := NewIndexFor(phpparser.OnlyGrammar(token.PHP74))
	uri := "file:///app/broken.php"
	idx.IndexFile(uri, "<?php\nfunction match($a) { return $a; }\n$x = ;\n")
	moveTo(t, idx, phpparser.GrammarsBetween(token.PHP74, token.Latest))
	_, _, candidates := idx.reparseCandidates()
	if len(candidates) != 1 || candidates[0].uri != uri {
		t.Fatalf("candidates = %+v, want the file with errors alone", candidates)
	}
	if !idx.ReparseForGrammars(func(uri, _ string) { t.Errorf("%s was swapped although it keeps PHP 7.4", uri) }) {
		t.Error("nothing newer arrived")
	}
	if idx.Lookup("match") == nil {
		t.Error("function match() stays declared")
	}
}

func TestReparseForGrammarsKeepsAnIDEHelperFileAHelper(t *testing.T) {
	idx := NewIndexFor(phpparser.OnlyGrammar(token.PHP74))
	model := "file:///app/Models/User.php"
	idx.IndexFile(model, "<?php\nclass User {}\n")
	helper := "file:///app/_ide_helper_models.php"
	idx.IndexIDEHelperFile(helper, "<?php\n/** @property string $email */\nclass User {}\nclass Readonly {}\n")
	if idx.Lookup("Readonly") == nil {
		t.Fatal("PHP 7.4 reads class Readonly")
	}
	moveTo(t, idx, phpparser.OnlyGrammar(token.PHP81))
	var reparsed []string
	idx.ReparseForGrammars(func(uri, _ string) { reparsed = append(reparsed, uri) })
	if !slices.Contains(reparsed, helper) {
		t.Fatalf("reparsed %v, want the helper among them", reparsed)
	}
	if idx.Lookup("Readonly") != nil {
		t.Error("readonly is a keyword on PHP 8.1")
	}
	if user := idx.Lookup("User"); user == nil || user.URI != model {
		t.Errorf("User = %+v, want the model's declaration", user)
	}
	if email := idx.Lookup("User::$email"); email == nil || !email.IsVirtual || email.URI != helper {
		t.Errorf("User::$email = %+v, want the helper's virtual property", email)
	}
}
