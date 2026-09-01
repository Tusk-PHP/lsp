package main

// eval.go — asks one server one question and grades the answer.
//
// Grading is deliberately shape-tolerant: servers phrase hover text
// differently, return Location vs LocationLink, hierarchical vs flat symbols.
// A case fails only when the *information* is missing or wrong, never because
// of formatting.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Status values for a graded case.
const (
	StatusPass        = "pass"
	StatusFail        = "fail"
	StatusUnsupported = "unsupported"
	StatusSkipped     = "skipped"
	StatusError       = "error"
)

// CaseResult is one server's graded answer to one case.
type CaseResult struct {
	ID        string                 `json:"id"`
	Feature   string                 `json:"feature"`
	Workspace string                 `json:"workspace"`
	Tags      []string               `json:"tags,omitempty"`
	Status    string                 `json:"status"`
	Detail    string                 `json:"detail,omitempty"`
	LatencyMs []float64              `json:"latencyMs,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// session is one live server bound to one workspace.
type session struct {
	client   *Client
	root     string
	caps     map[string]interface{}
	open     map[string]bool
	srcCache map[string]string
	timeout  time.Duration
	diagWait time.Duration
	sawDiags bool
}

func (s *session) source(rel string) (string, error) {
	if src, ok := s.srcCache[rel]; ok {
		return src, nil
	}
	b, err := os.ReadFile(filepath.Join(s.root, rel))
	if err != nil {
		return "", err
	}
	s.srcCache[rel] = string(b)
	return string(b), nil
}

func (s *session) uri(rel string) string { return pathToURI(filepath.Join(s.root, rel)) }

func (s *session) didOpen(rel string) error {
	uri := s.uri(rel)
	if s.open[uri] {
		return nil
	}
	src, err := s.source(rel)
	if err != nil {
		return err
	}
	if err := s.client.Notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": src,
		},
	}); err != nil {
		return err
	}
	s.open[uri] = true
	return nil
}

func (s *session) didClose(rel string) {
	uri := s.uri(rel)
	if !s.open[uri] {
		return
	}
	_ = s.client.Notify("textDocument/didClose", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	delete(s.open, uri)
}

// supports reports whether the server advertised the capability a feature needs.
func (s *session) supports(feature string) bool {
	key := map[string]string{
		"hover":           "hoverProvider",
		"definition":      "definitionProvider",
		"typeDefinition":  "typeDefinitionProvider",
		"implementation":  "implementationProvider",
		"references":      "referencesProvider",
		"completion":      "completionProvider",
		"documentSymbol":  "documentSymbolProvider",
		"workspaceSymbol": "workspaceSymbolProvider",
		"signatureHelp":   "signatureHelpProvider",
	}[feature]
	if key == "" {
		return true // diagnostics are pushed; there is no capability to check
	}
	v, ok := s.caps[key]
	if !ok || v == nil {
		return false
	}
	if b, isBool := v.(bool); isBool {
		return b
	}
	return true // options object counts as supported
}

// Run executes a case: warmup calls, then repeat timed calls, then grading.
func (s *session) Run(c Case, workspace string, warmup, repeat int) CaseResult {
	res := CaseResult{ID: c.ID, Feature: c.Feature, Workspace: workspace, Tags: c.Tags, Meta: map[string]interface{}{}}
	if !s.supports(c.Feature) {
		res.Status = StatusUnsupported
		res.Detail = "capability not advertised"
		return res
	}
	if c.Feature == "diagnostics" {
		return s.runDiagnostics(c, res, repeat)
	}
	if err := s.didOpen(c.File); err != nil {
		res.Status, res.Detail = StatusError, err.Error()
		return res
	}
	src, err := s.source(c.File)
	if err != nil {
		res.Status, res.Detail = StatusError, err.Error()
		return res
	}
	var pos Position
	if featureNeedsPosition(c.Feature) {
		pos, err = resolveCaret(src, c)
		if err != nil {
			res.Status, res.Detail = StatusError, err.Error()
			return res
		}
		res.Meta["position"] = fmt.Sprintf("%d:%d", pos.Line+1, pos.Character+1)
	}
	method, params := s.request(c, pos)

	for i := 0; i < warmup; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		_, _ = s.client.Call(ctx, method, params)
		cancel()
	}
	var last json.RawMessage
	for i := 0; i < repeat; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
		start := time.Now()
		raw, callErr := s.client.Call(ctx, method, params)
		elapsed := time.Since(start)
		cancel()
		if callErr != nil {
			res.Status, res.Detail = StatusError, callErr.Error()
			return res
		}
		res.LatencyMs = append(res.LatencyMs, float64(elapsed.Microseconds())/1000)
		last = raw
	}
	s.grade(c, last, &res)
	return res
}

func (s *session) request(c Case, pos Position) (string, map[string]interface{}) {
	uri := s.uri(c.File)
	doc := map[string]interface{}{"uri": uri}
	base := map[string]interface{}{"textDocument": doc, "position": pos}
	switch c.Feature {
	case "hover":
		return "textDocument/hover", base
	case "definition":
		return "textDocument/definition", base
	case "typeDefinition":
		return "textDocument/typeDefinition", base
	case "implementation":
		return "textDocument/implementation", base
	case "references":
		base["context"] = map[string]interface{}{"includeDeclaration": true}
		return "textDocument/references", base
	case "completion":
		base["context"] = map[string]interface{}{"triggerKind": 1}
		return "textDocument/completion", base
	case "signatureHelp":
		base["context"] = map[string]interface{}{"triggerKind": 1, "isRetrigger": false}
		return "textDocument/signatureHelp", base
	case "documentSymbol":
		return "textDocument/documentSymbol", map[string]interface{}{"textDocument": doc}
	case "workspaceSymbol":
		return "workspace/symbol", map[string]interface{}{"query": c.Query}
	}
	return "", nil
}

func (s *session) grade(c Case, raw json.RawMessage, res *CaseResult) {
	switch c.Feature {
	case "hover", "signatureHelp":
		text := normalizeSpace(flattenText(raw))
		res.Meta["text"] = truncate(text, 400)
		if text == "" {
			res.Status, res.Detail = StatusFail, "empty response"
			return
		}
		var missing, forbidden []string
		for _, want := range c.Expect.Contains {
			if !strings.Contains(text, want) {
				missing = append(missing, want)
			}
		}
		for _, no := range c.Expect.NotContains {
			if strings.Contains(text, no) {
				forbidden = append(forbidden, no)
			}
		}
		if len(missing) == 0 && len(forbidden) == 0 {
			res.Status = StatusPass
			return
		}
		res.Status = StatusFail
		res.Detail = strings.TrimSpace(fmt.Sprintf("missing %v unexpected %v", missing, forbidden))

	case "definition", "typeDefinition", "implementation":
		locs := parseLocations(raw)
		res.Meta["targets"] = summarizeLocations(locs, s.root, 3)
		if len(locs) == 0 {
			res.Status, res.Detail = StatusFail, "no target returned"
			return
		}
		for _, l := range locs {
			p := uriToPath(l.URI)
			if c.Expect.TargetFile != "" && !strings.HasSuffix(filepath.ToSlash(p), c.Expect.TargetFile) {
				continue
			}
			if c.Expect.TargetSymbol != "" && !lineContains(p, l.Range.Start.Line, c.Expect.TargetSymbol) {
				continue
			}
			res.Status = StatusPass
			return
		}
		res.Status = StatusFail
		res.Detail = fmt.Sprintf("no target matched %s / %q", c.Expect.TargetFile, c.Expect.TargetSymbol)

	case "completion":
		labels := parseCompletionLabels(raw)
		res.Meta["itemCount"] = len(labels)
		index := map[string]int{}
		for i, l := range labels {
			if _, seen := index[l]; !seen {
				index[l] = i
			}
		}
		var missing []string
		ranks := map[string]int{}
		for _, want := range c.Expect.Items {
			if i, ok := index[want]; ok {
				ranks[want] = i + 1
			} else {
				missing = append(missing, want)
			}
		}
		var forbidden []string
		for _, no := range c.Expect.Forbid {
			if _, ok := index[no]; ok {
				forbidden = append(forbidden, no)
			}
		}
		if len(ranks) > 0 {
			res.Meta["ranks"] = ranks
		}
		if len(missing) == 0 && len(forbidden) == 0 {
			res.Status = StatusPass
			return
		}
		res.Status = StatusFail
		res.Detail = strings.TrimSpace(fmt.Sprintf("missing %v unexpected %v (of %d items)", missing, forbidden, len(labels)))
		res.Meta["sampleItems"] = sample(labels, 20)

	case "documentSymbol", "workspaceSymbol":
		names := parseSymbolNames(raw)
		res.Meta["symbolCount"] = len(names)
		var missing []string
		for _, want := range c.Expect.Items {
			found := false
			for _, n := range names {
				if strings.Contains(n, want) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, want)
			}
		}
		if len(missing) == 0 {
			res.Status = StatusPass
			return
		}
		res.Status, res.Detail = StatusFail, fmt.Sprintf("missing %v (of %d symbols)", missing, len(names))
		res.Meta["sampleSymbols"] = sample(names, 20)

	case "references":
		locs := parseLocations(raw)
		res.Meta["refCount"] = len(locs)
		res.Meta["refFiles"] = summarizeLocations(locs, s.root, 5)
		if c.Expect.MinCount > 0 && len(locs) < c.Expect.MinCount {
			res.Status, res.Detail = StatusFail, fmt.Sprintf("%d references, want >= %d", len(locs), c.Expect.MinCount)
			return
		}
		if c.Expect.MaxCount > 0 && len(locs) > c.Expect.MaxCount {
			res.Status, res.Detail = StatusFail, fmt.Sprintf("%d references, want <= %d", len(locs), c.Expect.MaxCount)
			return
		}
		var missing []string
		for _, want := range c.Expect.Files {
			found := false
			for _, l := range locs {
				if strings.HasSuffix(filepath.ToSlash(uriToPath(l.URI)), want) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, want)
			}
		}
		if len(missing) == 0 {
			res.Status = StatusPass
			return
		}
		res.Status, res.Detail = StatusFail, fmt.Sprintf("no reference in %v", missing)

	default:
		res.Status, res.Detail = StatusError, "unknown feature "+c.Feature
	}
}

// runDiagnostics grades pushed diagnostics: close, reopen, wait for a publish.
func (s *session) runDiagnostics(c Case, res CaseResult, repeat int) CaseResult {
	src, err := s.source(c.File)
	if err != nil {
		res.Status, res.Detail = StatusError, err.Error()
		return res
	}
	var pos Position
	if c.Anchor != "" {
		if pos, err = resolveCaret(src, c); err != nil {
			res.Status, res.Detail = StatusError, err.Error()
			return res
		}
		res.Meta["position"] = fmt.Sprintf("%d:%d", pos.Line+1, pos.Character+1)
	}
	uri := s.uri(c.File)
	var last json.RawMessage
	var published bool
	for i := 0; i < repeat; i++ {
		s.didClose(c.File)
		s.client.ClearDiagnostics(uri)
		start := time.Now()
		if err := s.didOpen(c.File); err != nil {
			res.Status, res.Detail = StatusError, err.Error()
			return res
		}
		// Some servers only analyse on save. An editor sends this too, so
		// sending it keeps the comparison about analysis, not about which
		// notification a server happens to listen for.
		_ = s.client.Notify("textDocument/didSave", map[string]interface{}{
			"textDocument": map[string]interface{}{"uri": uri},
			"text":         src,
		})
		first, params, ok := s.waitForDiagnostics(uri, start)
		if !ok {
			continue
		}
		published = true
		s.sawDiags = true
		// Latency is time to the first publish — what the user sees. Grading
		// uses the settled payload, so a server that refines its answer is not
		// penalised for publishing early.
		res.LatencyMs = append(res.LatencyMs, float64(first.Microseconds())/1000)
		last = params
	}
	if !published {
		if c.Expect.Clean {
			res.Status, res.Detail = StatusPass, "no diagnostics published"
			return res
		}
		res.Status, res.Detail = StatusFail, "no diagnostics published within timeout"
		return res
	}
	items := parseDiagnostics(last)
	res.Meta["diagnosticCount"] = len(items)
	if c.Expect.Clean {
		// Only errors and warnings count as false positives. Hints and
		// information (unused import, style nudges) are opinions, not claims
		// that the code is wrong.
		var loud []diagnostic
		for _, d := range items {
			if d.Severity == 0 || d.Severity <= 2 {
				loud = append(loud, d)
			}
		}
		res.Meta["errorOrWarningCount"] = len(loud)
		if len(loud) == 0 {
			res.Status = StatusPass
			return res
		}
		res.Status = StatusFail
		res.Detail = fmt.Sprintf("expected none, got %d: %s", len(loud), truncate(loud[0].Message, 120))
		return res
	}
	for _, d := range items {
		if d.Range.Start.Line != pos.Line {
			continue
		}
		if c.Expect.Code != "" && !strings.Contains(strings.ToLower(d.Code), strings.ToLower(c.Expect.Code)) {
			continue
		}
		if !messageMatches(d.Message, c.Expect.MessageAny) {
			continue
		}
		res.Status = StatusPass
		res.Meta["matched"] = truncate(d.Message, 160)
		return res
	}
	res.Status = StatusFail
	res.Detail = fmt.Sprintf("no diagnostic on line %d matching code=%q message=%v (%d published)", pos.Line+1, c.Expect.Code, c.Expect.MessageAny, len(items))
	if len(items) > 0 {
		res.Meta["published"] = truncate(items[0].Message, 160)
	}
	return res
}

// messageMatches reports whether a diagnostic message uses any of the accepted
// wordings. An empty list accepts anything on the right line.
func messageMatches(message string, any []string) bool {
	if len(any) == 0 {
		return true
	}
	lower := strings.ToLower(message)
	for _, want := range any {
		if strings.Contains(lower, strings.ToLower(want)) {
			return true
		}
	}
	return false
}

// featureNeedsPosition reports whether a feature is asked at a caret.
func featureNeedsPosition(feature string) bool {
	switch feature {
	case "workspaceSymbol", "documentSymbol":
		return false
	}
	return true
}

// waitForDiagnostics returns the delay until the first publish for a URI and
// the payload once publishing has settled. Servers commonly publish twice —
// syntax first, semantics after — and some publish an empty list before their
// index is warm, so the settled payload is the fair one to grade.
func (s *session) waitForDiagnostics(uri string, after time.Time) (time.Duration, json.RawMessage, bool) {
	const settle = 1500 * time.Millisecond
	deadline := time.Now().Add(s.diagWait)
	var (
		first    time.Duration
		latest   json.RawMessage
		latestAt time.Time
		got      bool
	)
	for time.Now().Before(deadline) {
		params, at, ok := s.client.Diagnostics(uri)
		if ok && at.After(after) && at.After(latestAt) {
			if !got {
				first = at.Sub(after)
				got = true
			}
			latest, latestAt = params, at
		}
		if got && time.Since(latestAt) > settle {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	return first, latest, got
}

// ---------------------------------------------------------------------------
// Response shape normalizers
// ---------------------------------------------------------------------------

type lspRange struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type location struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

// parseLocations accepts Location, Location[], LocationLink[] or null.
func parseLocations(raw json.RawMessage) []location {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var single location
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return []location{single}
	}
	var list []struct {
		URI          string    `json:"uri"`
		Range        lspRange  `json:"range"`
		TargetURI    string    `json:"targetUri"`
		TargetRange  lspRange  `json:"targetRange"`
		TargetSelect *lspRange `json:"targetSelectionRange"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil
	}
	out := make([]location, 0, len(list))
	for _, l := range list {
		if l.TargetURI != "" {
			r := l.TargetRange
			if l.TargetSelect != nil {
				r = *l.TargetSelect
			}
			out = append(out, location{URI: l.TargetURI, Range: r})
			continue
		}
		if l.URI != "" {
			out = append(out, location{URI: l.URI, Range: l.Range})
		}
	}
	return out
}

func summarizeLocations(locs []location, root string, max int) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range locs {
		p := uriToPath(l.URI)
		if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
		key := fmt.Sprintf("%s:%d", filepath.ToSlash(p), l.Range.Start.Line+1)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
		if len(out) >= max {
			break
		}
	}
	return out
}

// parseCompletionLabels accepts CompletionList or CompletionItem[].
func parseCompletionLabels(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	type item struct {
		Label      string `json:"label"`
		FilterText string `json:"filterText"`
		InsertText string `json:"insertText"`
	}
	extract := func(items []item) []string {
		out := make([]string, 0, len(items))
		for _, it := range items {
			label := strings.TrimSpace(it.Label)
			// Trim decorations some servers add: "$var", "method()", "Class ".
			label = strings.TrimSuffix(label, "()")
			out = append(out, label)
		}
		return out
	}
	var list struct {
		Items []item `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err == nil && list.Items != nil {
		return extract(list.Items)
	}
	var arr []item
	if err := json.Unmarshal(raw, &arr); err == nil {
		return extract(arr)
	}
	return nil
}

// parseSymbolNames accepts DocumentSymbol[] (nested) or SymbolInformation[].
func parseSymbolNames(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var nodes []struct {
		Name          string            `json:"name"`
		ContainerName string            `json:"containerName"`
		Children      []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return nil
	}
	var out []string
	for _, n := range nodes {
		if n.ContainerName != "" {
			out = append(out, n.ContainerName+"\\"+n.Name, n.Name)
		} else {
			out = append(out, n.Name)
		}
		for _, child := range n.Children {
			out = append(out, parseSymbolNames(mustArray(child))...)
		}
	}
	return out
}

func mustArray(raw json.RawMessage) json.RawMessage {
	return json.RawMessage("[" + string(raw) + "]")
}

type diagnostic struct {
	Range    lspRange `json:"range"`
	Message  string   `json:"message"`
	Code     string   `json:"-"`
	RawCode  any      `json:"code"`
	Source   string   `json:"source"`
	Severity int      `json:"severity"`
}

func parseDiagnostics(raw json.RawMessage) []diagnostic {
	var p struct {
		Diagnostics []diagnostic `json:"diagnostics"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	for i := range p.Diagnostics {
		switch v := p.Diagnostics[i].RawCode.(type) {
		case string:
			p.Diagnostics[i].Code = v
		case float64:
			p.Diagnostics[i].Code = fmt.Sprintf("%.0f", v)
		}
	}
	return p.Diagnostics
}

// flattenText pulls every human-readable string out of a hover or signature
// help payload, whatever nesting the server chose.
func flattenText(raw json.RawMessage) string {
	var v interface{}
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(interface{})
	walk = func(node interface{}) {
		switch t := node.(type) {
		case string:
			sb.WriteString(t)
			sb.WriteString("\n")
		case []interface{}:
			for _, item := range t {
				walk(item)
			}
		case map[string]interface{}:
			// Prefer the documented text carriers; ignore metadata keys.
			for _, key := range []string{"contents", "value", "label", "documentation", "signatures", "parameters"} {
				if child, ok := t[key]; ok {
					walk(child)
				}
			}
		}
	}
	walk(v)
	return sb.String()
}

// sample returns at most n entries, for putting a readable slice of a large
// response into the report.
func sample(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return append(append([]string{}, items[:n]...), fmt.Sprintf("… +%d more", len(items)-n))
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func lineContains(path string, line int, want string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lines := strings.Split(string(b), "\n")
	// Allow a small window: servers disagree on whether a definition points at
	// the docblock, the attribute, or the declaration itself.
	for i := line - 2; i <= line+2; i++ {
		if i >= 0 && i < len(lines) && strings.Contains(lines[i], want) {
			return true
		}
	}
	return false
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}
