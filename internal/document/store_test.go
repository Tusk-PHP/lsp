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
	s.Save("file:///b.php", "<?php\n$c;\n")
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
