package main

// spec.go — declarative inputs: which servers to benchmark (servers.json) and
// which questions to ask them (suite/*.json).

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// ServerSpec describes how to launch and configure one language server.
type ServerSpec struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Homepage    string                 `json:"homepage"`
	License     string                 `json:"license"`
	Disabled    bool                   `json:"disabled"`
	Cmd         []string               `json:"cmd"`
	Env         map[string]string      `json:"env"`
	VersionCmd  []string               `json:"versionCmd"`
	InitOptions map[string]interface{} `json:"initializationOptions"`
	// ResetPaths are wiped before every run so no server benefits from a warm
	// on-disk cache the others do not have.
	ResetPaths []string `json:"resetPaths"`
	// ReadySignals are native "I finished indexing" notifications. They are
	// recorded when seen but never required — readiness is decided by probe.
	ReadySignals    []ReadySignal `json:"readySignals"`
	ReadyTimeoutSec int           `json:"readyTimeoutSec"`
	// SkipFeatures excludes features the server cannot reasonably be asked for
	// (recorded as "skipped", never as a failure).
	SkipFeatures []string `json:"skipFeatures"`
	Notes        string   `json:"notes"`
}

// ReadySignal matches a server-specific indexing-finished message.
type ReadySignal struct {
	Type    string `json:"type"` // "notification" | "logMessage"
	Method  string `json:"method"`
	Pattern string `json:"pattern"`

	re *regexp.Regexp
}

func (r *ReadySignal) compile() error {
	if r.Pattern == "" {
		return nil
	}
	re, err := regexp.Compile(r.Pattern)
	if err != nil {
		return fmt.Errorf("bad readySignal pattern %q: %w", r.Pattern, err)
	}
	r.re = re
	return nil
}

// Matches reports whether a notification satisfies this signal.
func (r *ReadySignal) Matches(n Notification) bool {
	switch r.Type {
	case "notification":
		return n.Method == r.Method
	case "logMessage":
		if n.Method != "window/logMessage" {
			return false
		}
		var p struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(n.Params, &p) != nil {
			return false
		}
		return r.re == nil || r.re.MatchString(p.Message)
	}
	return false
}

// Suite is one workspace plus the cases run against it.
type Suite struct {
	Workspace   string   `json:"workspace"`
	Root        string   `json:"root"`
	Description string   `json:"description"`
	OpenFiles   []string `json:"openFiles"`
	ReadyProbe  string   `json:"readyProbe"`
	Comment     string   `json:"comment"`
	Cases       []Case   `json:"cases"`

	file string
}

// Case is a single question put to every server at the same caret position.
type Case struct {
	ID      string `json:"id"`
	Feature string `json:"feature"`
	Desc    string `json:"desc"`
	File    string `json:"file"`

	// Caret location: the Nth occurrence of Anchor, advanced by Offset runes.
	Anchor     string `json:"anchor"`
	Occurrence int    `json:"occurrence"`
	Offset     *int   `json:"offset"`

	Query       string   `json:"query"` // workspace/symbol
	Comment     string   `json:"comment"`
	Expect      Expect   `json:"expect"`
	Tags        []string `json:"tags"`
	SkipServers []string `json:"skipServers"`
}

// Expect holds the assertions for a case. Only the fields relevant to the
// case's feature are consulted, and every check is deliberately loose enough to
// be answerable by servers that format their output differently.
type Expect struct {
	Contains    []string `json:"contains"`    // hover, signatureHelp
	NotContains []string `json:"notContains"` // hover, signatureHelp

	TargetFile   string `json:"targetFile"`   // definition-family: path suffix
	TargetSymbol string `json:"targetSymbol"` // text expected on the target line

	Items  []string `json:"items"`  // completion, documentSymbol, workspaceSymbol
	Forbid []string `json:"forbid"` // completion items that must NOT appear

	MinCount int      `json:"minCount"` // references
	MaxCount int      `json:"maxCount"` // references
	Files    []string `json:"files"`    // references: path suffixes expected

	// diagnostics: the expected code substring, and message wording that any one
	// server might use — servers phrase the same finding very differently, so a
	// match against any entry counts.
	Code       string   `json:"code"`
	MessageAny []string `json:"messageAny"`
	Clean      bool     `json:"clean"` // file must produce no error/warning
}

// LoadServers reads servers.json and compiles its patterns.
func LoadServers(path string) ([]ServerSpec, error) {
	var specs []ServerSpec
	if err := readJSON(path, &specs); err != nil {
		return nil, err
	}
	for i := range specs {
		for j := range specs[i].ReadySignals {
			if err := specs[i].ReadySignals[j].compile(); err != nil {
				return nil, fmt.Errorf("server %s: %w", specs[i].ID, err)
			}
		}
		if specs[i].ReadyTimeoutSec == 0 {
			specs[i].ReadyTimeoutSec = 300
		}
	}
	return specs, nil
}

// LoadSuites reads every *.json under dir.
func LoadSuites(dir string) ([]Suite, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var suites []Suite
	for _, e := range entries {
		var s Suite
		if err := readJSON(e, &s); err != nil {
			return nil, fmt.Errorf("%s: %w", e, err)
		}
		s.file = e
		seen := map[string]bool{}
		for _, c := range s.Cases {
			if seen[c.ID] {
				return nil, fmt.Errorf("%s: duplicate case id %q", e, c.ID)
			}
			seen[c.ID] = true
		}
		suites = append(suites, s)
	}
	return suites, nil
}

func readJSON(path string, v interface{}) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// expandVars substitutes {{root}}, {{workspace}}, {{storage}} and {{tools}}.
func expandVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

func expandSlice(in []string, vars map[string]string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = expandVars(s, vars)
	}
	return out
}

func expandMap(in map[string]interface{}, vars map[string]string) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		switch t := v.(type) {
		case string:
			out[k] = expandVars(t, vars)
		case map[string]interface{}:
			out[k] = expandMap(t, vars)
		default:
			out[k] = v
		}
	}
	return out
}

// Position is a zero-based LSP position (Character counts UTF-16 units).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// resolveCaret converts a case's anchor into a concrete position in src.
func resolveCaret(src string, c Case) (Position, error) {
	if c.Anchor == "" {
		return Position{}, fmt.Errorf("case %s: missing anchor", c.ID)
	}
	occurrence := c.Occurrence
	if occurrence <= 0 {
		occurrence = 1
	}
	idx := -1
	from := 0
	for i := 0; i < occurrence; i++ {
		rel := strings.Index(src[from:], c.Anchor)
		if rel < 0 {
			return Position{}, fmt.Errorf("case %s: anchor %q occurrence %d not found in file", c.ID, c.Anchor, occurrence)
		}
		idx = from + rel
		from = idx + 1
	}
	offset := utf8.RuneCountInString(c.Anchor)
	if c.Offset != nil {
		offset = *c.Offset
	}
	// Advance offset runes from the anchor start.
	byteIdx := idx
	for i := 0; i < offset && byteIdx < len(src); i++ {
		_, size := utf8.DecodeRuneInString(src[byteIdx:])
		byteIdx += size
	}
	return byteOffsetToPosition(src, byteIdx), nil
}

func byteOffsetToPosition(src string, byteIdx int) Position {
	if byteIdx > len(src) {
		byteIdx = len(src)
	}
	line := strings.Count(src[:byteIdx], "\n")
	lineStart := strings.LastIndexByte(src[:byteIdx], '\n') + 1
	prefix := src[lineStart:byteIdx]
	return Position{Line: line, Character: len(utf16.Encode([]rune(prefix)))}
}

func pathToURI(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	u := url.URL{Scheme: "file", Path: abs}
	return u.String()
}

func uriToPath(u string) string {
	if !strings.HasPrefix(u, "file://") {
		return u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return strings.TrimPrefix(u, "file://")
	}
	return parsed.Path
}
