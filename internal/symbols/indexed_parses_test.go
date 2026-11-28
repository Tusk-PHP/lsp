package symbols

import (
	"strconv"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
)

func TestIndexedParseIsTheLatestTextOfTheLastFilesIndexed(t *testing.T) {
	idx := NewIndex()
	first := "<?php\nclass Room {}\n"
	idx.IndexFile("file:///room.php", first)
	if parsed, file, ok := idx.IndexedParse("file:///room.php", first); !ok || parsed.Tree == nil || len(file.Classes) != 1 {
		t.Fatalf("the text just indexed: ok %v, file %+v", ok, file)
	}
	if _, _, ok := idx.IndexedParse("file:///room.php", first+"// edited\n"); ok {
		t.Error("a text the index hasn't read reuses its parse")
	}
	if _, _, ok := idx.IndexedParse("file:///other.php", first); ok {
		t.Error("another file's text reuses this file's parse")
	}

	second := "<?php\nclass Hall {}\n"
	idx.IndexFile("file:///room.php", second)
	if _, _, ok := idx.IndexedParse("file:///room.php", first); ok {
		t.Error("the text before the file was indexed again reuses its old parse")
	}
	if _, file, ok := idx.IndexedParse("file:///room.php", second); !ok || file.Classes[0].Name != "Hall" {
		t.Errorf("the text indexed again: ok %v, file %+v", ok, file)
	}

	idx.IndexIDEHelperFile("file:///room.php", second)
	if _, _, ok := idx.IndexedParse("file:///room.php", second); ok {
		t.Error("a file indexed as an IDE helper reuses the parse indexed before")
	}
	idx.IndexFile("file:///room.php", first)

	for i := range keptParses {
		idx.IndexFile("file:///f"+strconv.Itoa(i)+".php", "<?php\nfunction f"+strconv.Itoa(i)+"() {}\n")
	}
	if _, _, ok := idx.IndexedParse("file:///room.php", first); ok {
		t.Errorf("more than %d files later, the parse is still kept", keptParses)
	}
	last := "file:///f" + strconv.Itoa(keptParses-1) + ".php"
	lastText := "<?php\nfunction f" + strconv.Itoa(keptParses-1) + "() {}\n"
	if _, _, ok := idx.IndexedParse(last, lastText); !ok {
		t.Error("the last file indexed has no parse")
	}
	idx.RemoveFile(last)
	if _, _, ok := idx.IndexedParse(last, lastText); ok {
		t.Error("a removed file keeps its parse")
	}
}

func TestKeepParseKeepsOnlyTheIndexedText(t *testing.T) {
	idx := NewIndex()
	text := "<?php\nclass Room {}\n"
	idx.IndexFile("file:///room.php", text)
	for i := range keptParses {
		idx.IndexFile("file:///f"+strconv.Itoa(i)+".php", "<?php\n")
	}
	parsed, file := phpparserParse(text)
	idx.KeepParse("file:///room.php", text+"// edited\n", parsed, file)
	idx.KeepParse("file:///other.php", text, parsed, file)
	if _, _, ok := idx.IndexedParse("file:///room.php", text); ok {
		t.Fatal("a parse of another text or file was kept for the indexed text")
	}
	idx.KeepParse("file:///room.php", text, parsed, file)
	if _, got, ok := idx.IndexedParse("file:///room.php", text); !ok || got != file {
		t.Errorf("the request's parse of the indexed text: ok %v", ok)
	}
	idx.IndexIDEHelperFile("file:///room.php", text)
	idx.KeepParse("file:///room.php", text, parsed, file)
	if _, _, ok := idx.IndexedParse("file:///room.php", text); ok {
		t.Error("a parse is kept for a file indexed as an IDE helper")
	}
}

func TestKeepParseOfAKeptFileTakesItsSlot(t *testing.T) {
	idx := NewIndex()
	first, room := "<?php\nclass First {}\n", "<?php\nclass Room {}\n"
	idx.IndexFile("file:///first.php", first)
	idx.IndexFile("file:///room.php", room)
	for i := range keptParses - 2 {
		idx.IndexFile("file:///f"+strconv.Itoa(i)+".php", "<?php\n")
	}
	parsed, file := phpparserParse(room)
	idx.KeepParse("file:///room.php", room, parsed, file)
	if _, got, ok := idx.IndexedParse("file:///room.php", room); !ok || got != file {
		t.Errorf("the request's parse replaces the kept one: ok %v", ok)
	}
	if _, _, ok := idx.IndexedParse("file:///first.php", first); !ok {
		t.Error("keeping a parse of a file already kept evicted another file's")
	}
}

func phpparserParse(text string) (phpparser.Parsed, *parser.FileNode) {
	parsed := parser.ParseIn(text, phpparser.AnyGrammar())
	return parsed, parser.FileNodeOf(text, parsed.Tree)
}
