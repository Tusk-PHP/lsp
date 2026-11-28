package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const positionsShop = `<?php
namespace App;

class Shop
{
    /* Größe 😀 漢字 */ public function total(): int { return 1; }
}
`

const positionsView = "<p>Größe 😀 漢字</p> @include('partials.nav')\n"

const positionsUse = `<?php
use App\Shop;
$shop = new Shop(); $msg = 'Größe 😀 漢字'; $shop->total(); missingFn();
// Véase 😀 https://a.example/x
`

type positionsCase struct {
	t      *testing.T
	h      *lspHarness
	enc    string
	shop   string
	view   string
	use    string
	useSrc string
}

func newPositionsCase(t *testing.T, offered []string) *positionsCase {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	shopPath := filepath.Join(root, "src", "Shop.php")
	viewPath := filepath.Join(root, "resources", "views", "page.blade.php")
	for path, text := range map[string]string{
		shopPath: positionsShop,
		viewPath: positionsView,
		filepath.Join(root, "resources", "views", "partials", "nav.blade.php"): "<nav></nav>\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t)
	t.Cleanup(h.close)
	caps := map[string]interface{}{}
	if offered != nil {
		caps["general"] = map[string]interface{}{"positionEncodings": offered}
	}
	init := h.readResponse(h.send("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "capabilities": caps, "processId": nil}))
	enc, _ := init["result"].(map[string]interface{})["capabilities"].(map[string]interface{})["positionEncoding"].(string)
	h.notify("initialized", map[string]interface{}{})
	waitFor(t, "the index to be ready", h.server.index.Ready)
	c := &positionsCase{t: t, h: h, enc: enc, shop: symbols.PathToURI(shopPath), view: symbols.PathToURI(viewPath), use: symbols.PathToURI(filepath.Join(root, "src", "use.php")), useSrc: positionsUse}
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": c.use, "languageId": "php", "version": 1, "text": positionsUse},
	})
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": c.view, "languageId": "blade", "version": 1, "text": positionsView},
	})
	return c
}

func (c *positionsCase) col(text string, line int, before string) float64 {
	c.t.Helper()
	return c.colFrom(text, line, 0, before)
}

func (c *positionsCase) colFrom(text string, line, from int, before string) float64 {
	c.t.Helper()
	l := strings.Split(text, "\n")[line]
	i := strings.Index(l[from:], before)
	if i < 0 {
		c.t.Fatalf("%q is not on line %d", before, line)
	}
	i += from
	if c.enc == "utf-8" {
		return float64(i)
	}
	return float64(len(utf16.Encode([]rune(l[:i]))))
}

func (c *positionsCase) lineEnd(text string, line int) float64 {
	l := strings.Split(text, "\n")[line]
	if c.enc == "utf-8" {
		return float64(len(l))
	}
	return float64(len(utf16.Encode([]rune(l))))
}

func (c *positionsCase) at(line int, before string) map[string]interface{} {
	return map[string]interface{}{"line": line, "character": c.col(c.useSrc, line, before)}
}

func (c *positionsCase) request(method string, params map[string]interface{}) interface{} {
	c.t.Helper()
	params["textDocument"] = map[string]interface{}{"uri": c.use}
	return c.h.readResponse(c.h.send(method, params))["result"]
}

func (c *positionsCase) wantRange(what string, got interface{}, text string, line int, start, end string) {
	c.t.Helper()
	r, _ := got.(map[string]interface{})
	s, _ := r["start"].(map[string]interface{})
	e, _ := r["end"].(map[string]interface{})
	if s == nil || e == nil {
		c.t.Errorf("%s: no range in %v", what, got)
		return
	}
	from := strings.Index(strings.Split(text, "\n")[line], start)
	want := [4]float64{float64(line), c.col(text, line, start), float64(line), c.colFrom(text, line, max(from, 0), end)}
	have := [4]float64{toFloat(s["line"]), toFloat(s["character"]), toFloat(e["line"]), toFloat(e["character"])}
	if have != want {
		c.t.Errorf("%s: range %v, want %v", what, have, want)
	}
}

func toFloat(v interface{}) float64 {
	f, _ := v.(float64)
	return f
}

func TestPositionsAfterNonASCIIText(t *testing.T) {
	for _, tc := range []struct {
		name    string
		offered []string
		want    string
	}{
		{"default", nil, "utf-16"},
		{"utf-16 only", []string{"utf-16"}, "utf-16"},
		{"utf-8 offered", []string{"utf-16", "utf-8"}, "utf-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newPositionsCase(t, tc.offered)
			if c.enc != tc.want {
				t.Fatalf("positionEncoding %q, want %q", c.enc, tc.want)
			}
			c.checkHover()
			c.checkDefinition()
			c.checkReferences()
			c.checkRename()
			c.checkSemanticTokens()
			c.checkSelectionRange()
			c.checkDocumentLink()
			c.checkDiagnostics()
			c.checkCompletionAfterChange()
			c.checkIncrementalChange()
		})
	}
}

func (c *positionsCase) checkHover() {
	resp := c.h.readResponse(c.h.send("textDocument/hover", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": c.view},
		"position":     map[string]interface{}{"line": 0, "character": c.col(positionsView, 0, "nclude")},
	}))
	hover, _ := resp["result"].(map[string]interface{})
	c.wantRange("hover", hover["range"], positionsView, 0, "@include", "(")
}

func (c *positionsCase) checkDefinition() {
	loc, _ := c.request("textDocument/definition", map[string]interface{}{"position": c.at(2, "total(); missing")}).(map[string]interface{})
	if loc["uri"] != c.shop {
		c.t.Fatalf("definition: %v", loc)
	}
	c.wantRange("definition", loc["range"], positionsShop, 5, "total(): int", "(): int")
}

func (c *positionsCase) checkReferences() {
	refs, _ := c.request("textDocument/references", map[string]interface{}{
		"position": c.at(2, "total(); missing"),
		"context":  map[string]interface{}{"includeDeclaration": true},
	}).([]interface{})
	found := map[string]bool{}
	for _, r := range refs {
		loc := r.(map[string]interface{})
		uri := loc["uri"].(string)
		found[uri] = true
		if uri == c.shop {
			c.wantRange("reference in Shop.php", loc["range"], positionsShop, 5, "total(): int", "(): int")
		} else {
			c.wantRange("reference in use.php", loc["range"], c.useSrc, 2, "total(); missing", "(); missing")
		}
	}
	if !found[c.shop] || !found[c.use] {
		c.t.Errorf("references: %v", refs)
	}
}

func (c *positionsCase) checkRename() {
	edit, _ := c.request("textDocument/rename", map[string]interface{}{"position": c.at(2, "total(); missing"), "newName": "sum"}).(map[string]interface{})
	changes, _ := edit["changes"].(map[string]interface{})
	shop, _ := changes[c.shop].([]interface{})
	use, _ := changes[c.use].([]interface{})
	if len(shop) != 1 || len(use) != 1 {
		c.t.Fatalf("rename: %v", edit)
	}
	c.wantRange("rename in Shop.php", shop[0].(map[string]interface{})["range"], positionsShop, 5, "total(): int", "(): int")
	c.wantRange("rename in use.php", use[0].(map[string]interface{})["range"], c.useSrc, 2, "total(); missing", "(); missing")
}

func (c *positionsCase) checkSemanticTokens() {
	result, _ := c.request("textDocument/semanticTokens/full", map[string]interface{}{}).(map[string]interface{})
	data, _ := result["data"].([]interface{})
	line, char := 0.0, 0.0
	want := map[[3]float64]bool{
		{2, c.col(c.useSrc, 2, "'Größe"), c.col(c.useSrc, 2, "; $shop->") - c.col(c.useSrc, 2, "'Größe")}: false,
	}
	for i := 0; i+4 < len(data); i += 5 {
		dl, dc, length := toFloat(data[i]), toFloat(data[i+1]), toFloat(data[i+2])
		if dl > 0 {
			line, char = line+dl, dc
		} else {
			char += dc
		}
		key := [3]float64{line, char, length}
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for tok, seen := range want {
		if !seen {
			c.t.Errorf("semantic tokens: no token at line %v, column %v, length %v in %v", tok[0], tok[1], tok[2], data)
		}
	}
}

func (c *positionsCase) checkSelectionRange() {
	ranges, _ := c.request("textDocument/selectionRange", map[string]interface{}{"positions": []interface{}{c.at(2, "tal(); missing")}}).([]interface{})
	if len(ranges) != 1 {
		c.t.Fatalf("selection ranges: %v", ranges)
	}
	c.wantRange("selection range", ranges[0].(map[string]interface{})["range"], c.useSrc, 2, "total(); missing", "(); missing")
}

func (c *positionsCase) checkDocumentLink() {
	links, _ := c.request("textDocument/documentLink", map[string]interface{}{}).([]interface{})
	if len(links) != 1 {
		c.t.Fatalf("document links: %v", links)
	}
	r := links[0].(map[string]interface{})["range"].(map[string]interface{})
	start, end := r["start"].(map[string]interface{}), r["end"].(map[string]interface{})
	if start["character"] != c.col(c.useSrc, 3, "https://") || end["character"] != c.lineEnd(c.useSrc, 3) {
		c.t.Errorf("document link: %v, want columns %v to %v", r, c.col(c.useSrc, 3, "https://"), c.lineEnd(c.useSrc, 3))
	}
}

func (c *positionsCase) checkDiagnostics() {
	c.h.notify("textDocument/didSave", map[string]interface{}{"textDocument": map[string]interface{}{"uri": c.use}})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		note := c.h.readNotification("textDocument/publishDiagnostics", 10*time.Second)
		params := note["params"].(map[string]interface{})
		if params["uri"] != c.use {
			continue
		}
		for _, d := range params["diagnostics"].([]interface{}) {
			diag := d.(map[string]interface{})
			if diag["code"] == "unknown-function" {
				c.wantRange("diagnostic", diag["range"], c.useSrc, 2, "missingFn", "();")
				return
			}
		}
	}
	c.t.Error("no unknown-function diagnostic was published")
}

func (c *positionsCase) checkCompletionAfterChange() {
	c.useSrc = "<?php\nuse App\\Shop;\n$shop = new Shop(); $msg = 'Größe 😀 漢字'; $shop->to\n"
	c.h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": c.use, "version": 2},
		"contentChanges": []interface{}{map[string]interface{}{"text": c.useSrc}},
	})
	items, _ := c.request("textDocument/completion", map[string]interface{}{
		"position": map[string]interface{}{"line": 2, "character": c.lineEnd(c.useSrc, 2)},
	}).([]interface{})
	for _, it := range items {
		if it.(map[string]interface{})["label"] == "total" {
			return
		}
	}
	c.t.Errorf("completion after non-ASCII text does not offer total: %v", items)
}

func TestDefinitionInComposerJSONReadsTheClientColumn(t *testing.T) {
	h := initHarnessWithOpts(t,
		map[string]interface{}{"window": map[string]interface{}{"showDocument": map[string]interface{}{"support": true}}},
		map[string]interface{}{"composer": map[string]interface{}{"openOnDefinition": true}})
	defer h.close()
	const uri = "file:///tmp/positions/composer.json"
	const source = `{"description": "Größe 😀 漢字", "require": {"monolog/monolog": "^3"}}` + "\n"
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "languageId": "json", "version": 1, "text": source},
	})
	c := &positionsCase{t: t, enc: "utf-16"}

	defID := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 0, "character": c.col(source, 0, "monolog/")},
	})
	defResp, showDoc := drainMessages(h, defID, 5*time.Second)
	if defResp == nil {
		t.Fatal("timed out waiting for the definition response")
	}
	if showDoc == nil {
		t.Fatal("definition on a package name after non-ASCII text sent no window/showDocument")
	}
	params, _ := showDoc["params"].(map[string]interface{})
	if got, want := params["uri"], "https://packagist.org/packages/monolog/monolog"; got != want {
		t.Errorf("window/showDocument uri = %v, want %s", got, want)
	}
}

func TestPositionsOfAFileWithoutTextAreSentAsBytes(t *testing.T) {
	var logged strings.Builder
	s := NewServer(strings.NewReader(""), io.Discard, log.New(&logged, "", 0))
	uri := symbols.PathToURI(filepath.Join(t.TempDir(), "missing.php"))

	pos := protocol.Position{Line: 2, Character: 7}
	if got := s.positions().posOut(uri, pos); got != pos {
		t.Errorf("posOut on a file with no text = %+v, want the byte position %+v", got, pos)
	}
	if !strings.Contains(logged.String(), uri) {
		t.Errorf("no log line names the file without text; logged %q", logged.String())
	}
}

func TestCopyReferenceCommandConversion(t *testing.T) {
	command := func(arg string) *protocol.Command {
		return &protocol.Command{Command: protocol.CommandCopyReference, Arguments: []json.RawMessage{json.RawMessage(`"App\\Foo"`), json.RawMessage(arg)}}
	}

	t.Run("a position before any non-ASCII text is kept", func(t *testing.T) {
		in := command(`{"line":0,"character":3}`)
		got, converted := copyPositions().convertCommand(copyURI, in)
		if converted || got != in {
			t.Errorf("convertCommand = %+v, %v; want the same command, not converted", got, converted)
		}
	})

	t.Run("an argument that is not a position panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "copyReference argument is not a position") {
				t.Errorf("recovered %v, want a panic naming the copyReference argument", r)
			}
		}()
		copyPositions().convertCommand(copyURI, command(`"not a position"`))
	})
}

func (c *positionsCase) checkIncrementalChange() {
	start := c.col(c.useSrc, 2, "$shop->to")
	end := c.lineEnd(c.useSrc, 2)
	want := "<?php\nuse App\\Shop;\n$shop = new Shop(); $msg = 'Größe 😀 漢字'; $shop->total();\n// é\n"
	c.h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": c.use, "version": 3},
		"contentChanges": []interface{}{
			map[string]interface{}{
				"range": map[string]interface{}{"start": map[string]interface{}{"line": 2, "character": start}, "end": map[string]interface{}{"line": 2, "character": end}},
				"text":  "$shop->total();\n// x",
			},
			map[string]interface{}{
				"range": map[string]interface{}{"start": map[string]interface{}{"line": 3, "character": 3}, "end": map[string]interface{}{"line": 3, "character": 4}},
				"text":  "é",
			},
		},
	})
	c.useSrc = want
	loc, _ := c.request("textDocument/definition", map[string]interface{}{"position": c.at(2, "total();")}).(map[string]interface{})
	if got, _ := c.h.server.docs.Text(c.use); got != want {
		c.t.Fatalf("after the incremental change the server has\n%q\nwant\n%q", got, want)
	}
	if loc["uri"] != c.shop {
		c.t.Errorf("definition after the incremental change: %v", loc)
	}
}
