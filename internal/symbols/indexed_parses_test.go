package symbols

import (
	"strconv"
	"testing"
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
