package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const bladeTestModel = `<?php

namespace App\Models;

class User
{
    public string $name = '';

    public function fullName(string $separator = ' ', bool $short = false): string
    {
        return $this->name;
    }
}
`

const bladeTestView = `<div class="profile">
@php
    /** @var \App\Models\User $user */
@endphp
<h1>{{ $user->fullName() }}</h1>
@foreach($users as $member)
    <p>{{ $member->name }}</p>
@endforeach
</div>
`

// bladeWorkspace starts a server on a workspace with a model and a view
// that uses it, and opens both.
func bladeWorkspace(t *testing.T, view string) (h *lspHarness, modelURI, viewURI string) {
	t.Helper()
	root := t.TempDir()
	modelPath := filepath.Join(root, "app", "Models", "User.php")
	viewPath := filepath.Join(root, "resources", "views", "profile.blade.php")
	for path, text := range map[string]string{modelPath: bladeTestModel, viewPath: view} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h = newHarness(t)
	id := h.send("initialize", map[string]interface{}{
		"rootUri": "file://" + root, "capabilities": map[string]interface{}{}, "processId": nil,
	})
	h.readResponse(id)
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(500 * time.Millisecond)

	modelURI = "file://" + filepath.ToSlash(modelPath)
	viewURI = "file://" + filepath.ToSlash(viewPath)
	for uri, text := range map[string]string{modelURI: bladeTestModel, viewURI: view} {
		lang := "php"
		if uri == viewURI {
			lang = "blade"
		}
		h.notify("textDocument/didOpen", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri, "languageId": lang, "version": 1, "text": text},
		})
	}
	time.Sleep(200 * time.Millisecond)
	return h, modelURI, viewURI
}

// posOf returns the line and column of the first needle in text, plus offset.
func posOf(t *testing.T, text, needle string, offset int) map[string]interface{} {
	t.Helper()
	for i, line := range strings.Split(text, "\n") {
		if k := strings.Index(line, needle); k >= 0 {
			return map[string]interface{}{"line": i, "character": k + offset}
		}
	}
	t.Fatalf("%q not found", needle)
	return nil
}

func request(t *testing.T, h *lspHarness, method, uri string, pos map[string]interface{}, extra map[string]interface{}, out interface{}) {
	t.Helper()
	params := map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}, "position": pos}
	for k, v := range extra {
		params[k] = v
	}
	resp := h.readResponse(h.send(method, params))
	if resp["error"] != nil {
		t.Fatalf("%s returned error: %v", method, resp["error"])
	}
	raw, _ := json.Marshal(resp["result"])
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s result %s: %v", method, raw, err)
	}
}

type testRange struct {
	Start struct{ Line, Character int }
	End   struct{ Line, Character int }
}

type testLocation struct {
	URI   string
	Range testRange
}

func TestBladeHoverInEcho(t *testing.T) {
	h, _, viewURI := bladeWorkspace(t, bladeTestView)
	defer h.close()

	var hover struct {
		Contents struct{ Value string }
		Range    *testRange
	}
	request(t, h, "textDocument/hover", viewURI, posOf(t, bladeTestView, "fullName()", 2), nil, &hover)
	if !strings.Contains(hover.Contents.Value, "fullName") {
		t.Fatalf("expected hover on fullName, got %q", hover.Contents.Value)
	}
	if hover.Range != nil && hover.Range.Start.Line != 4 {
		t.Fatalf("hover range should be on the view's line 4, got %+v", *hover.Range)
	}

	// HTML around the PHP has nothing to hover.
	var none interface{}
	request(t, h, "textDocument/hover", viewURI, posOf(t, bladeTestView, "profile", 2), nil, &none)
	if none != nil {
		t.Fatalf("expected no hover in HTML, got %v", none)
	}
}

func TestBladeCompletionInEcho(t *testing.T) {
	view := strings.Replace(bladeTestView, "{{ $user->fullName() }}", "{{ $user->fu }}", 1)
	h, _, viewURI := bladeWorkspace(t, view)
	defer h.close()

	var items []struct{ Label string }
	request(t, h, "textDocument/completion", viewURI, posOf(t, view, "$user->fu", len("$user->fu")), nil, &items)
	for _, it := range items {
		if it.Label == "fullName" {
			return
		}
	}
	t.Fatalf("expected fullName in completion, got %+v", items)
}

func TestBladeCompletionQualifiesClassNames(t *testing.T) {
	view := strings.Replace(bladeTestView, "{{ $user->fullName() }}", "{{ new Use }}", 1)
	h, _, viewURI := bladeWorkspace(t, view)
	defer h.close()

	var items []struct {
		Label               string
		InsertText          string
		AdditionalTextEdits []interface{}
	}
	request(t, h, "textDocument/completion", viewURI, posOf(t, view, "new Use", len("new Use")), nil, &items)
	for _, it := range items {
		if it.Label != "User" {
			continue
		}
		if len(it.AdditionalTextEdits) != 0 || !strings.HasPrefix(it.InsertText, `\App\Models\User`) {
			t.Fatalf("a view has no place for an import: expected a qualified name, got %+v", it)
		}
		return
	}
	t.Fatalf("expected User in completion, got %+v", items)
}

func TestBladeDefinition(t *testing.T) {
	h, modelURI, viewURI := bladeWorkspace(t, bladeTestView)
	defer h.close()

	var loc testLocation
	request(t, h, "textDocument/definition", viewURI, posOf(t, bladeTestView, "fullName()", 2), nil, &loc)
	if loc.URI != modelURI {
		t.Fatalf("expected definition in the model, got %+v", loc)
	}

	// A variable goes to its class, typed by the @var in the @php block.
	loc = testLocation{}
	request(t, h, "textDocument/definition", viewURI, posOf(t, bladeTestView, "$user->", 2), nil, &loc)
	if loc.URI != modelURI || loc.Range.Start.Line != 4 {
		t.Fatalf("expected $user to go to class User, got %+v", loc)
	}

	// HTML has no definition.
	var none interface{}
	request(t, h, "textDocument/definition", viewURI, posOf(t, bladeTestView, "profile", 2), nil, &none)
	if none != nil {
		t.Fatalf("expected no definition in HTML, got %v", none)
	}
}

func TestBladeSignatureHelp(t *testing.T) {
	h, _, viewURI := bladeWorkspace(t, bladeTestView)
	defer h.close()

	var help struct {
		Signatures []struct{ Label string }
	}
	request(t, h, "textDocument/signatureHelp", viewURI, posOf(t, bladeTestView, "fullName()", len("fullName(")), nil, &help)
	if len(help.Signatures) == 0 || !strings.Contains(help.Signatures[0].Label, "$separator") {
		t.Fatalf("expected fullName's signature, got %+v", help)
	}
}

func TestBladeReferencesFromPHP(t *testing.T) {
	h, modelURI, viewURI := bladeWorkspace(t, bladeTestView)
	defer h.close()

	var locs []testLocation
	request(t, h, "textDocument/references", modelURI, posOf(t, bladeTestModel, "fullName", 2), nil, &locs)
	want := posOf(t, bladeTestView, "fullName()", 0)
	for _, l := range locs {
		if l.URI == viewURI && l.Range.Start.Line == want["line"] && l.Range.Start.Character == want["character"] {
			return
		}
	}
	t.Fatalf("expected the use in the view at %v, got %+v", want, locs)
}

func TestBladeRenameFromView(t *testing.T) {
	h, modelURI, viewURI := bladeWorkspace(t, bladeTestView)
	defer h.close()

	var edit struct {
		Changes map[string][]struct {
			Range   testRange
			NewText string
		}
	}
	request(t, h, "textDocument/rename", viewURI, posOf(t, bladeTestView, "fullName()", 2), map[string]interface{}{"newName": "displayName"}, &edit)
	if len(edit.Changes[modelURI]) == 0 {
		t.Fatalf("expected the declaration renamed, got %+v", edit.Changes)
	}
	viewEdits := edit.Changes[viewURI]
	want := posOf(t, bladeTestView, "fullName()", 0)
	if len(viewEdits) != 1 || viewEdits[0].Range.Start.Line != want["line"] || viewEdits[0].Range.Start.Character != want["character"] || viewEdits[0].Range.End.Character != want["character"].(int)+len("fullName") {
		t.Fatalf("expected one edit at %v in the view, got %+v", want, viewEdits)
	}
}

func TestBladeRenameLoopVariable(t *testing.T) {
	h, _, viewURI := bladeWorkspace(t, bladeTestView)
	defer h.close()

	var edit struct {
		Changes map[string][]struct {
			Range   testRange
			NewText string
		}
	}
	request(t, h, "textDocument/rename", viewURI, posOf(t, bladeTestView, "$member->name", 2), map[string]interface{}{"newName": "$person"}, &edit)
	edits := edit.Changes[viewURI]
	if len(edits) != 2 {
		t.Fatalf("expected both uses of $member renamed, got %+v", edit.Changes)
	}
	lines := strings.Split(bladeTestView, "\n")
	for _, e := range edits {
		if got := lines[e.Range.Start.Line][e.Range.Start.Character:e.Range.End.Character]; got != "$member" {
			t.Fatalf("edit covers %q, not $member", got)
		}
	}
}

func TestBladeInlayHints(t *testing.T) {
	view := strings.Replace(bladeTestView, "{{ $user->fullName() }}", "{{ $user->fullName(' - ', true) }}", 1)
	h, _, viewURI := bladeWorkspace(t, view)
	defer h.close()

	resp := h.readResponse(h.send("textDocument/inlayHint", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": viewURI},
		"range":        map[string]interface{}{"start": map[string]int{"line": 0, "character": 0}, "end": map[string]int{"line": 20, "character": 0}},
	}))
	raw, _ := json.Marshal(resp["result"])
	var hints []struct {
		Position struct{ Line, Character int }
		Label    string
	}
	_ = json.Unmarshal(raw, &hints)
	want := posOf(t, view, "' - '", 0)
	for _, hint := range hints {
		if strings.Contains(hint.Label, "separator") {
			if hint.Position.Line != want["line"] || hint.Position.Character != want["character"] {
				t.Fatalf("expected the hint at %v, got %+v", want, hint)
			}
			return
		}
	}
	t.Fatalf("expected a separator: hint, got %s", raw)
}
