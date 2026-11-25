package document

import (
	"strings"
	"testing"
)

func TestStoreKeepsTextAndVersion(t *testing.T) {
	s := NewStore()
	s.Open("file:///b.php", 1, "<?php\n$a;\n")
	s.Open("file:///a.php", 3, "<?php\n")
	text, err := s.Change("file:///b.php", 2, UTF16, []Change{{Range: at(1, 1, 1, 2), Text: "b"}})
	if err != nil || text != "<?php\n$b;\n" {
		t.Fatalf("Change = %q, %v", text, err)
	}
	if doc, _ := s.Get("file:///b.php"); doc.Version != 2 || doc.Text != text {
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
	s := NewStore()
	first := s.Open("file:///a.php", 1, "<?php\n")
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
	again := s.Open("file:///a.php", 3, "<?php\n")
	if s.Current(third) || !s.Current(again) {
		t.Error("a snapshot from before a close and reopen with the same version and text counts as current")
	}
}

func TestStoreRejectsChangesItCannotApply(t *testing.T) {
	s := NewStore()
	if _, err := s.Change("file:///x.php", 2, UTF16, []Change{{Text: "x"}}); err == nil || !strings.Contains(err.Error(), "not open") {
		t.Errorf("change to a closed document: %v", err)
	}
	s.Open("file:///x.php", 1, "<?php\n")
	if _, err := s.Change("file:///x.php", 2, UTF16, []Change{{Range: at(9, 0, 9, 0), Text: "x"}}); err == nil || !strings.Contains(err.Error(), "file:///x.php, version 2") {
		t.Errorf("change past the end: %v", err)
	}
	if doc, _ := s.Get("file:///x.php"); doc.Version != 1 || doc.Text != "<?php\n" {
		t.Errorf("a rejected change left %+v", doc)
	}
}
