package laravel

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// TranslationResolver completes and finds the translation keys of a Laravel
// app, from lang/ and resources/lang/. The keys are read once and kept until
// FilesChanged reports a change to them.
type TranslationResolver struct {
	rootPath string
	entries  atomic.Pointer[func() translations]
}

type translations struct {
	entries []TranslationEntry
	byKey   map[string]int
}

type TranslationEntry struct {
	Key    string
	Locale string
	File   string
	URI    string
	Range  textpos.Range
}

type TranslationStringContext struct {
	Key     string
	Partial string
	Quote   string
}

func NewTranslationResolver(rootPath string) *TranslationResolver {
	r := &TranslationResolver{rootPath: rootPath}
	r.invalidate()
	return r
}

// WatchGlobs are the files besides PHP sources that the Laravel features
// read and a client should report changes to: JSON translations.
func WatchGlobs() []string {
	return []string{"**/lang/*.json"}
}

func translationDirs(rootPath string) []string {
	return []string{filepath.Join(rootPath, "lang"), filepath.Join(rootPath, "resources", "lang")}
}

// FilesChanged forgets the keys read when one of paths, created, changed or
// deleted, is a translation file or config/app.php, which sets the
// preferred locale.
func (r *TranslationResolver) FilesChanged(paths []string) {
	if r == nil || r.rootPath == "" {
		return
	}
	watched := append(translationDirs(r.rootPath), r.appConfigPath())
	for _, path := range paths {
		if touchesAny(path, watched) {
			r.invalidate()
			return
		}
	}
}

func (r *TranslationResolver) invalidate() {
	load := sync.OnceValue(r.read)
	r.entries.Store(&load)
}

func (r *TranslationResolver) translations() translations {
	return (*r.entries.Load())()
}

func DetectTranslationContext(line string, cursor int) *TranslationStringContext {
	if cursor < 0 {
		return nil
	}
	if cursor > len(line) {
		cursor = len(line)
	}

	patterns := []string{
		"__(",
		"trans(",
		"trans_choice(",
		"@lang(",
		"Lang::get(",
		"Lang::choice(",
	}

	var best *TranslationStringContext
	bestStart := -1
	for _, pattern := range patterns {
		searchFrom := 0
		for {
			idx := strings.Index(line[searchFrom:], pattern)
			if idx < 0 {
				break
			}
			idx += searchFrom
			ctx := detectTranslationContextAt(line, cursor, idx+len(pattern))
			if ctx != nil && idx >= bestStart {
				best = ctx
				bestStart = idx
			}
			searchFrom = idx + len(pattern)
		}
	}

	return best
}

func detectTranslationContextAt(line string, cursor, argStart int) *TranslationStringContext {
	i := argStart
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || (line[i] != '\'' && line[i] != '"') {
		return nil
	}

	quote := line[i]
	valueStart := i + 1
	valueEnd := len(line)
	closed := false
	for j := valueStart; j < len(line); j++ {
		if line[j] == quote && line[j-1] != '\\' {
			valueEnd = j
			closed = true
			break
		}
	}

	if cursor < valueStart {
		return nil
	}
	if closed && cursor > valueEnd {
		return nil
	}

	full := line[valueStart:valueEnd]
	partialEnd := min(cursor, valueEnd)
	if partialEnd < valueStart {
		partialEnd = valueStart
	}

	return &TranslationStringContext{
		Key:     full,
		Partial: line[valueStart:partialEnd],
		Quote:   string(quote),
	}
}

// Matching returns the translation keys that start with ctx.Partial,
// ignoring case: completion offers them.
func (r *TranslationResolver) Matching(ctx *TranslationStringContext) []TranslationEntry {
	if r == nil || ctx == nil {
		return nil
	}
	entries := r.translations().entries
	if len(entries) == 0 {
		return nil
	}
	// Not nil once there are keys, even when none matches.
	partial := strings.ToLower(ctx.Partial)
	matches := make([]TranslationEntry, 0, len(entries))
	for _, entry := range entries {
		if partial != "" && !strings.HasPrefix(strings.ToLower(entry.Key), partial) {
			continue
		}
		matches = append(matches, entry)
	}
	return matches
}

// Definition returns the entry that defines key in the preferred locale, or
// nil.
func (r *TranslationResolver) Definition(key string) *TranslationEntry {
	if r == nil || key == "" {
		return nil
	}
	t := r.translations()
	i, ok := t.byKey[key]
	if !ok {
		return nil
	}
	entry := t.entries[i]
	return &entry
}

func (r *TranslationResolver) read() translations {
	entries := r.discoverEntries()
	if len(entries) == 0 {
		return translations{}
	}

	preferredLocale := r.defaultLocale()
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		aPref := localeRank(a.Locale, preferredLocale)
		bPref := localeRank(b.Locale, preferredLocale)
		if aPref != bPref {
			return aPref < bPref
		}
		if a.Locale != b.Locale {
			return a.Locale < b.Locale
		}
		return a.File < b.File
	})

	uniq := make([]TranslationEntry, 0, len(entries))
	byKey := make(map[string]int, len(entries))
	for _, entry := range entries {
		if _, seen := byKey[entry.Key]; seen {
			continue
		}
		byKey[entry.Key] = len(uniq)
		uniq = append(uniq, entry)
	}
	return translations{entries: uniq, byKey: byKey}
}

func localeRank(locale, preferred string) int {
	switch {
	case preferred != "" && locale == preferred:
		return 0
	case locale == "en":
		return 1
	default:
		return 2
	}
}

func (r *TranslationResolver) defaultLocale() string {
	data, err := os.ReadFile(r.appConfigPath())
	if err != nil {
		return "en"
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "'locale'") && !strings.Contains(trimmed, "\"locale\"") {
			continue
		}
		if loc := parseQuotedValueAfterArrow(trimmed); loc != "" {
			return loc
		}
	}
	return "en"
}

func (r *TranslationResolver) appConfigPath() string {
	return filepath.Join(r.rootPath, "config", "app.php")
}

func parseQuotedValueAfterArrow(line string) string {
	arrow := strings.Index(line, "=>")
	if arrow < 0 {
		return ""
	}
	value := strings.TrimSpace(line[arrow+2:])
	if len(value) < 2 {
		return ""
	}
	quote := value[0]
	if quote != '\'' && quote != '"' {
		return ""
	}
	for i := 1; i < len(value); i++ {
		if value[i] == quote && value[i-1] != '\\' {
			return value[1:i]
		}
	}
	return ""
}

func (r *TranslationResolver) discoverEntries() []TranslationEntry {
	if r.rootPath == "" {
		return nil
	}

	var entries []TranslationEntry
	for _, baseDir := range translationDirs(r.rootPath) {
		dirEntries, err := os.ReadDir(baseDir)
		if err != nil {
			continue
		}
		for _, entry := range dirEntries {
			path := filepath.Join(baseDir, entry.Name())
			switch {
			case entry.IsDir():
				entries = append(entries, discoverPHPTranslationEntries(path)...)
			case strings.HasSuffix(entry.Name(), ".json"):
				entries = append(entries, discoverJSONTranslationEntries(path)...)
			}
		}
	}
	return entries
}

func discoverPHPTranslationEntries(localeDir string) []TranslationEntry {
	locale := filepath.Base(localeDir)
	var entries []TranslationEntry

	_ = filepath.Walk(localeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		rel, err := filepath.Rel(localeDir, path)
		if err != nil {
			return nil
		}
		prefix := strings.TrimSuffix(filepath.ToSlash(rel), ".php")
		prefix = strings.ReplaceAll(prefix, "/", ".")
		entries = append(entries, parsePHPTranslationFile(path, locale, prefix)...)
		return nil
	})

	return entries
}

func parsePHPTranslationFile(path, locale, prefix string) []TranslationEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	src := string(data)
	open := strings.Index(src, "[")
	if open < 0 {
		return nil
	}

	parser := &phpTranslationParser{
		src:        src,
		path:       path,
		uri:        pathToURI(path),
		lineStarts: lineStarts(src),
	}
	parser.parseArray(open, prefix)
	for i := range parser.entries {
		parser.entries[i].Locale = locale
	}
	return parser.entries
}

func discoverJSONTranslationEntries(path string) []TranslationEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	locale := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	uri := pathToURI(path)
	src := string(data)

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	entries := make([]TranslationEntry, 0, len(keys))
	for _, key := range keys {
		rng := jsonKeyRange(src, key)
		entries = append(entries, TranslationEntry{
			Key:    key,
			Locale: locale,
			File:   filepath.Base(path),
			URI:    uri,
			Range:  rng,
		})
	}
	return entries
}

func jsonKeyRange(src, key string) textpos.Range {
	quoted, _ := json.Marshal(key)
	lines := strings.Split(src, "\n")
	for lineNo, line := range lines {
		idx := strings.Index(line, string(quoted))
		if idx < 0 {
			continue
		}
		return textpos.Range{
			Start: textpos.Position{Line: lineNo, Character: idx + 1},
			End:   textpos.Position{Line: lineNo, Character: idx + len(string(quoted)) - 1},
		}
	}
	return textpos.Range{}
}

type phpTranslationParser struct {
	src        string
	path       string
	uri        string
	lineStarts []int
	entries    []TranslationEntry
}

func (p *phpTranslationParser) parseArray(idx int, prefix string) int {
	if idx < 0 || idx >= len(p.src) || p.src[idx] != '[' {
		return idx
	}
	idx++
	for idx < len(p.src) {
		idx = p.skipIgnorable(idx)
		if idx >= len(p.src) {
			return idx
		}
		if p.src[idx] == ']' {
			return idx + 1
		}

		key, keyStart, keyEnd, next, ok := p.parseQuotedString(idx)
		if !ok {
			return p.skipValue(idx)
		}
		idx = p.skipIgnorable(next)
		if idx+1 >= len(p.src) || p.src[idx] != '=' || p.src[idx+1] != '>' {
			return p.skipValue(idx)
		}
		idx += 2
		idx = p.skipIgnorable(idx)

		fullKey := prefix + "." + key
		if prefix == "" {
			fullKey = key
		}

		if idx < len(p.src) && p.src[idx] == '[' {
			idx = p.parseArray(idx, fullKey)
		} else {
			p.entries = append(p.entries, TranslationEntry{
				Key:   fullKey,
				File:  filepath.Base(p.path),
				URI:   p.uri,
				Range: p.rangeForOffsets(keyStart, keyEnd),
			})
			idx = p.skipValue(idx)
		}

		idx = p.skipIgnorable(idx)
		if idx < len(p.src) && p.src[idx] == ',' {
			idx++
		}
	}
	return idx
}

func (p *phpTranslationParser) parseQuotedString(idx int) (value string, start, end, next int, ok bool) {
	if idx >= len(p.src) || (p.src[idx] != '\'' && p.src[idx] != '"') {
		return "", 0, 0, idx, false
	}
	quote := p.src[idx]
	start = idx + 1
	for i := start; i < len(p.src); i++ {
		if p.src[i] == quote && p.src[i-1] != '\\' {
			return p.src[start:i], start, i, i + 1, true
		}
	}
	return "", 0, 0, idx, false
}

func (p *phpTranslationParser) skipIgnorable(idx int) int {
	for idx < len(p.src) {
		switch p.src[idx] {
		case ' ', '\t', '\r', '\n', ',':
			idx++
		case '/':
			if idx+1 < len(p.src) && p.src[idx+1] == '/' {
				idx += 2
				for idx < len(p.src) && p.src[idx] != '\n' {
					idx++
				}
				continue
			}
			if idx+1 < len(p.src) && p.src[idx+1] == '*' {
				idx += 2
				for idx+1 < len(p.src) && !(p.src[idx] == '*' && p.src[idx+1] == '/') {
					idx++
				}
				if idx+1 < len(p.src) {
					idx += 2
				}
				continue
			}
			return idx
		default:
			return idx
		}
	}
	return idx
}

func (p *phpTranslationParser) skipValue(idx int) int {
	depthParen := 0
	depthBrace := 0
	depthBracket := 0
	for idx < len(p.src) {
		switch p.src[idx] {
		case '\'', '"':
			_, _, _, next, ok := p.parseQuotedString(idx)
			if !ok {
				return idx
			}
			idx = next
			continue
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case '[':
			depthBracket++
		case ']':
			if depthBracket == 0 && depthParen == 0 && depthBrace == 0 {
				return idx
			}
			if depthBracket > 0 {
				depthBracket--
			}
		case ',':
			if depthParen == 0 && depthBrace == 0 && depthBracket == 0 {
				return idx
			}
		}
		idx++
	}
	return idx
}

func (p *phpTranslationParser) rangeForOffsets(start, end int) textpos.Range {
	return textpos.Range{
		Start: offsetPosition(p.lineStarts, start),
		End:   offsetPosition(p.lineStarts, end),
	}
}

func lineStarts(src string) []int {
	starts := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func offsetPosition(starts []int, offset int) textpos.Position {
	line := sort.Search(len(starts), func(i int) bool { return starts[i] > offset }) - 1
	if line < 0 {
		line = 0
	}
	return textpos.Position{
		Line:      line,
		Character: offset - starts[line],
	}
}

func pathToURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
