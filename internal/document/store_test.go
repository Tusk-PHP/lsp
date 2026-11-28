package document

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestStoreKeepsTextAndVersion(t *testing.T) {
	s := NewStore(symbols.URIToPath)
	s.Open("file:///b.php", 1, "<?php\n$a;\n", false)
	s.Open("file:///a.php", 3, "<?php\n", false)
	changed, err := s.Change("file:///b.php", 2, UTF16, []Change{{Range: at(1, 1, 1, 2), Text: "b"}})
	if err != nil || changed.Text != "<?php\n$b;\n" || changed.Version != 2 {
		t.Fatalf("Change = %+v, %v", changed, err)
	}
	if doc, _ := s.Get("file:///b.php"); doc != changed {
		t.Errorf("after the change: %+v", doc)
	}
	if !s.Save("file:///b.php", "<?php\n$c;\n") {
		t.Fatal("saving an open document was refused")
	}
	if doc, _ := s.Get("file:///b.php"); doc.Version != 2 || doc.Text != "<?php\n$c;\n" {
		t.Errorf("a save replaces the text and keeps the version: %+v", doc)
	}
	all := s.All()
	if len(all) != 2 || all[0].URI != "file:///a.php" || all[1].URI != "file:///b.php" {
		t.Errorf("All = %+v, want both documents by URI", all)
	}
	s.Close("file:///a.php")
	if _, ok := s.Text("file:///a.php"); ok {
		t.Error("a closed document is still open")
	}
	if s.Save("file:///a.php", "<?php\n") {
		t.Error("saving a closed document reopened it")
	}
	if _, ok := s.Text("file:///a.php"); ok {
		t.Error("a save opened a closed document")
	}
}

func TestStoreCurrentFollowsEveryWriteNotTheText(t *testing.T) {
	s := NewStore(symbols.URIToPath)
	first := s.Open("file:///a.php", 1, "<?php\n", false)
	if !s.Current(first) {
		t.Fatal("a fresh snapshot is not current")
	}
	if _, err := s.Change("file:///a.php", 2, UTF16, []Change{{Text: "<?php\n$a;\n"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Change("file:///a.php", 3, UTF16, []Change{{Text: "<?php\n"}}); err != nil {
		t.Fatal(err)
	}
	if s.Current(first) {
		t.Error("version 1 counts as current after version 3, whose text is the same")
	}
	third, _ := s.Get("file:///a.php")
	s.Save("file:///a.php", "<?php\n")
	if !s.Current(third) {
		t.Error("saving the same text made the snapshot stale")
	}
	s.Close("file:///a.php")
	again := s.Open("file:///a.php", 3, "<?php\n", false)
	if s.Current(third) || !s.Current(again) {
		t.Error("a snapshot from before a close and reopen with the same version and text counts as current")
	}
}

func TestStoreRejectsChangesItCannotApply(t *testing.T) {
	s := NewStore(symbols.URIToPath)
	if _, err := s.Change("file:///x.php", 2, UTF16, []Change{{Text: "x"}}); !errors.Is(err, ErrNotOpen) || !strings.Contains(err.Error(), "file:///x.php, version 2") {
		t.Errorf("change to a closed document: %v", err)
	}
	s.Open("file:///x.php", 1, "<?php\n", false)
	if _, err := s.Change("file:///x.php", 2, UTF16, []Change{{Range: at(-1, 0, 0, 0), Text: "x"}}); err == nil || !strings.Contains(err.Error(), "file:///x.php, version 2") || errors.Is(err, ErrDiverged) {
		t.Errorf("a negative position: %v", err)
	}
	if doc, _ := s.Get("file:///x.php"); doc.Version != 1 || doc.Text != "<?php\n" || !doc.Diverged() {
		t.Errorf("a rejected change left %+v, want the last text that applied, marked diverged", doc)
	}
}

func TestStoreRefusesRangeEditsAfterARejectedChange(t *testing.T) {
	const uri = "file:///a.php"
	s := NewStore(symbols.URIToPath)
	s.Open(uri, 1, "<?php\nfoo();\n", false)
	if _, err := s.Change(uri, 2, UTF16, []Change{{Range: at(1, 3, 1, 1), Text: "x"}}); err == nil || errors.Is(err, ErrDiverged) {
		t.Fatalf("the first rejected change: %v, want the reason it can't apply", err)
	}
	if _, err := s.Change(uri, 3, UTF16, []Change{{Range: at(1, 0, 1, 0), Text: "bar();"}}); !errors.Is(err, ErrDiverged) {
		t.Fatalf("a range edit to the diverged text: %v, want ErrDiverged", err)
	}
	disk, ok := s.ShowDiverged(uri, "<?php\nfrom_disk();\n")
	if !ok || disk.Text != "<?php\nfrom_disk();\n" || !disk.Diverged() || !s.Current(disk) {
		t.Fatalf("ShowDiverged = %+v, %v", disk, ok)
	}
	if _, err := s.Change(uri, 4, UTF16, []Change{{Range: at(1, 0, 1, 0), Text: "x"}}); !errors.Is(err, ErrDiverged) {
		t.Fatalf("a range edit to the text read from disk: %v, want ErrDiverged", err)
	}

	doc, err := s.Change(uri, 5, UTF16, []Change{
		{Range: at(7, 0, 7, 0), Text: "lost"},
		{Text: "<?php\nfoo();\n"},
		{Range: at(1, 0, 1, 0), Text: "bar();"},
	})
	if err != nil || doc.Text != "<?php\nbar();foo();\n" || doc.Version != 5 || doc.Diverged() {
		t.Fatalf("a full text, then a range edit: %+v, %v", doc, err)
	}
	if doc, err := s.Change(uri, 6, UTF16, []Change{{Range: at(1, 0, 1, 6), Text: ""}}); err != nil || doc.Text != "<?php\nfoo();\n" {
		t.Errorf("a range edit after the full text: %+v, %v", doc, err)
	}
	if _, ok := s.ShowDiverged(uri, "disk"); ok {
		t.Error("ShowDiverged replaced the text of a document that is in sync")
	}
}

func TestStoreOnlyAReopenOrTheFullTextEndsADivergence(t *testing.T) {
	const uri = "file:///a.php"
	diverge := func(s *Store) {
		t.Helper()
		if _, err := s.Change(uri, 2, UTF16, []Change{{Range: at(-1, 0, 0, 0)}}); err == nil {
			t.Fatal("the negative position applied")
		}
	}
	s := NewStore(symbols.URIToPath)
	s.Open(uri, 1, "<?php\n", false)
	diverge(s)
	s.Open(uri, 1, "<?php\n", false)
	if doc, err := s.Change(uri, 2, UTF16, []Change{{Range: at(1, 0, 1, 0), Text: "a();"}}); err != nil || doc.Text != "<?php\na();" {
		t.Errorf("a range edit after a reopen: %+v, %v", doc, err)
	}

	diverge(s)
	if !s.Save(uri, "<?php\nsaved();\n") {
		t.Fatal("the save was refused")
	}
	if doc, _ := s.Get(uri); doc.Text != "<?php\nsaved();\n" || !doc.Diverged() {
		t.Errorf("after a save the diverged document is %+v, want the saved text, still diverged", doc)
	}
	if _, err := s.Change(uri, 3, UTF16, []Change{{Range: at(1, 0, 1, 0), Text: "b();"}}); !errors.Is(err, ErrDiverged) {
		t.Errorf("a range edit after a save: %v, want ErrDiverged", err)
	}
}

func TestStoreFindsAnOpenDocumentByPathAndKnowsWhetherItIsSaved(t *testing.T) {
	s := NewStore(symbols.URIToPath)
	s.Open("file:///srv/my%20app/a.php", 1, "<?php\n", false)
	s.Open("file:///srv/my app/a.php", 1, "<?php\n", false)
	if uri, open := s.URIAt("/srv/my app/a.php"); !open || uri != "file:///srv/my app/a.php" {
		t.Errorf("the path finds the URI opened last: %q, %v", uri, open)
	}
	if s.Close("file:///srv/my%20app/a.php") {
		t.Error("a document opened with its disk text closes saved")
	}
	if uri, open := s.URIAt("/srv/my app/a.php"); !open || uri != "file:///srv/my app/a.php" {
		t.Errorf("closing one URI of a path keeps the other open under it: %q, %v", uri, open)
	}

	if _, err := s.Change("file:///srv/my app/a.php", 2, UTF16, []Change{{Text: "<?php\n$a;\n"}}); err != nil {
		t.Fatal(err)
	}
	if !s.MarkSaved("file:///srv/my app/a.php") {
		t.Fatal("marking an open document saved was refused")
	}
	if _, err := s.Change("file:///srv/my app/a.php", 3, UTF16, []Change{{Text: "<?php\n$b;\n"}}); err != nil {
		t.Fatal(err)
	}
	if !s.Close("file:///srv/my app/a.php") {
		t.Error("an edit after the save leaves the document unsaved")
	}
	if _, open := s.URIAt("/srv/my app/a.php"); open {
		t.Error("closing the last URI of a path frees the path")
	}

	s.Open("file:///b.php", 1, "<?php\n", true)
	s.Save("file:///b.php", "<?php\n")
	if s.Close("file:///b.php") {
		t.Error("a save with its text leaves the document saved")
	}
	if s.MarkSaved("file:///b.php") {
		t.Error("a closed document was marked saved")
	}
}
