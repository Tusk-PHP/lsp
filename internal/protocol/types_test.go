package protocol

import (
	"encoding/json"
	"testing"
)

// A document change is written as whichever of its variants is set.
func TestDocumentChangeMarshalJSON(t *testing.T) {
	version := 2
	for _, c := range []struct {
		change DocumentChange
		want   string
	}{
		{DocumentChange{TextDocumentEdit: &TextDocumentEdit{TextDocument: VersionedTextDocumentIdentifier{URI: "file:///a.php", Version: &version}}},
			`{"textDocument":{"uri":"file:///a.php","version":2},"edits":null}`},
		{DocumentChange{RenameFile: &RenameFile{Kind: "rename", OldURI: "file:///a.php", NewURI: "file:///b.php"}},
			`{"kind":"rename","oldUri":"file:///a.php","newUri":"file:///b.php"}`},
		{DocumentChange{CreateFile: &CreateFile{Kind: "create", URI: "file:///c.php"}}, `{"kind":"create","uri":"file:///c.php"}`},
		{DocumentChange{DeleteFile: &DeleteFile{Kind: "delete", URI: "file:///d.php"}}, `{"kind":"delete","uri":"file:///d.php"}`},
		{DocumentChange{}, `null`},
	} {
		got, err := json.Marshal(c.change)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.want {
			t.Errorf("got %s, want %s", got, c.want)
		}
	}
}
