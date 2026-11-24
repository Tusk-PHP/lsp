package lsp

import (
	"strings"
	"testing"
)

func TestDeclarationInBladeViews(t *testing.T) {
	h, _, uri := viewsWorkspace(t, pageViews(), "page.blade.php")
	defer h.close()
	var locs []testLocation
	request(t, h, "textDocument/declaration", uri, posOf(t, bladeTestPage, "partials.nav", 2), nil, &locs)
	if len(locs) != 1 || !strings.HasSuffix(locs[0].URI, "partials/nav.blade.php") {
		t.Errorf("a view name's declaration is its file, got %+v", locs)
	}

	const view = `@php
    /** @var \App\Models\Admin $admin */
@endphp
<h1>{{ $admin->fullName() }}</h1>
<div class="profile"></div>
`
	const admin = `<?php

namespace App\Models;

interface HasFullName
{
    public function fullName(string $separator = ' ', bool $short = false): string;
}

class Admin extends User implements HasFullName
{
    public function fullName(string $separator = ' ', bool $short = false): string
    {
        return 'admin';
    }
}
`
	h2, _, viewURI := bladeWorkspace(t, view)
	defer h2.close()
	adminURI := strings.TrimSuffix(viewURI, "resources/views/profile.blade.php") + "app/Models/Admin.php"
	h2.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": adminURI, "languageId": "php", "version": 1, "text": admin},
	})
	var loc testLocation
	request(t, h2, "textDocument/declaration", viewURI, posOf(t, view, "fullName", 2), nil, &loc)
	if loc.URI != adminURI || loc.Range.Start.Line != 6 {
		t.Errorf("a method called in a view is declared by the interface its model implements, got %+v", loc)
	}
	request(t, h2, "textDocument/definition", viewURI, posOf(t, view, "fullName", 2), nil, &loc)
	if loc.URI != adminURI || loc.Range.Start.Line != 11 {
		t.Errorf("its definition stays the model's method, got %+v", loc)
	}
	resp := h2.readResponse(h2.send("textDocument/declaration", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": viewURI},
		"position":     posOf(t, view, "profile", 1),
	}))
	if resp["result"] != nil {
		t.Errorf("HTML has no declaration, got %v", resp["result"])
	}
}

func TestDeclarationRejectsInvalidParams(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	h.readResponse(h.send("initialize", initParams()))
	resp := h.readResponse(h.send("textDocument/declaration", "not an object"))
	errObj, _ := resp["error"].(map[string]interface{})
	if errObj == nil || errObj["code"] != float64(-32602) {
		t.Errorf("got %v", resp)
	}
}
