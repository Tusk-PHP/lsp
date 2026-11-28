package symfony

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// TranslationEntry is a single discovered translation key with its location.
type TranslationEntry struct {
	Key    string
	Domain string
	Locale string
	File   string
	URI    string
	Range  textpos.Range
}

// TranslationContext holds the context detected at the cursor position inside
// a Symfony translation call.
type TranslationContext struct {
	Key     string // full key typed so far (may include partial)
	Partial string // text before the cursor inside the string
	Domain  string // domain argument, if present and resolved
	Quote   string // quote character used (' or ")
}

// TranslationResolver discovers and indexes Symfony translation keys from the
// translations/ directory. It is nil-safe throughout.
type TranslationResolver struct {
	rootPath string
}

// NewTranslationResolver creates a resolver for the given project root.
func NewTranslationResolver(rootPath string) *TranslationResolver {
	return &TranslationResolver{rootPath: rootPath}
}

// DetectSymfonyTranslationContext detects whether the cursor is inside a
// Symfony translation function call and returns context for completion/definition.
// Recognised patterns:
//
//	$translator->trans('key', [], 'domain')
//	$this->trans('key')
//	t('key')
//	new TranslatableMessage('key')
func DetectSymfonyTranslationContext(line string, cursor int) *TranslationContext {
	if cursor < 0 {
		return nil
	}
	if cursor > len(line) {
		cursor = len(line)
	}

	patterns := []string{
		"->trans(",
		"->trans\t(",
		"t(",
		"TranslatableMessage(",
	}

	var best *TranslationContext
	bestStart := -1
	for _, pattern := range patterns {
		searchFrom := 0
		for {
			idx := strings.Index(line[searchFrom:], pattern)
			if idx < 0 {
				break
			}
			idx += searchFrom
			argStart := idx + len(pattern)
			if !translationCallStartsAt(line, idx, pattern) {
				searchFrom = argStart
				continue
			}
			ctx := detectSymfonyTranslationContextAt(line, cursor, argStart)
			if ctx != nil && idx >= bestStart {
				best = ctx
				bestStart = idx
			}
			searchFrom = idx + len(pattern)
		}
	}
	return best
}

// translationCallStartsAt reports whether the pattern found at line[idx] is
// the whole name of the call: `t(` is Symfony's t() function, not the end of
// get(, count( or $this->t(, and `TranslatableMessage(` not the end of
// another class name.
func translationCallStartsAt(line string, idx int, pattern string) bool {
	if pattern[0] == '-' || idx == 0 {
		return true
	}
	c := line[idx-1]
	if c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
		return false
	}
	if pattern == "t(" && (c == '>' || c == ':') {
		return false
	}
	return true
}

func detectSymfonyTranslationContextAt(line string, cursor, argStart int) *TranslationContext {
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
		if line[j] == quote && (j == 0 || line[j-1] != '\\') {
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
	partialEnd := cursor
	if partialEnd > valueEnd {
		partialEnd = valueEnd
	}
	if partialEnd < valueStart {
		partialEnd = valueStart
	}

	// Try to extract the domain from the third argument of trans('key', [], 'domain').
	domain := ""
	if closed {
		rest := line[valueEnd+1:]
		domain = extractThirdStringArg(rest)
	}

	return &TranslationContext{
		Key:     full,
		Partial: line[valueStart:partialEnd],
		Domain:  domain,
		Quote:   string(quote),
	}
}

// extractThirdStringArg tries to extract the third quoted string argument from
// text after the first argument's closing quote. It handles the pattern
// , $params, 'domain' by skipping two commas.
func extractThirdStringArg(rest string) string {
	commaCount := 0
	i := 0
	for i < len(rest) && commaCount < 2 {
		ch := rest[i]
		if ch == '\'' || ch == '"' {
			// Skip this string
			q := ch
			i++
			for i < len(rest) {
				if rest[i] == '\\' {
					i += 2
					continue
				}
				if rest[i] == q {
					i++
					break
				}
				i++
			}
			commaCount++
			continue
		}
		if ch == ',' {
			commaCount++
		}
		i++
	}
	// Now look for the next quoted string
	for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t') {
		i++
	}
	if i >= len(rest) || (rest[i] != '\'' && rest[i] != '"') {
		return ""
	}
	q := rest[i]
	start := i + 1
	for j := start; j < len(rest); j++ {
		if rest[j] == q && (j == 0 || rest[j-1] != '\\') {
			return rest[start:j]
		}
	}
	return ""
}

// Matching returns the translation keys that start with ctx.Partial,
// ignoring case, in ctx.Domain when it is set: completion offers them.
func (r *TranslationResolver) Matching(ctx *TranslationContext) []TranslationEntry {
	if r == nil || ctx == nil {
		return nil
	}
	partial := strings.ToLower(ctx.Partial)
	var matches []TranslationEntry
	for _, entry := range r.uniqueEntries() {
		if ctx.Domain != "" && entry.Domain != ctx.Domain {
			continue
		}
		if partial != "" && !strings.HasPrefix(strings.ToLower(entry.Key), partial) {
			continue
		}
		matches = append(matches, entry)
	}
	return matches
}

// Definition returns the entry that defines key, in any domain, or nil.
func (r *TranslationResolver) Definition(key string) *TranslationEntry {
	return r.DefinitionInDomain(key, "")
}

// DefinitionInDomain is like Definition but restricts to a specific domain
// when domain isn't empty.
func (r *TranslationResolver) DefinitionInDomain(key, domain string) *TranslationEntry {
	if r == nil || key == "" {
		return nil
	}
	for _, entry := range r.uniqueEntries() {
		if entry.Key == key && (domain == "" || entry.Domain == domain) {
			return &entry
		}
	}
	return nil
}

func (r *TranslationResolver) uniqueEntries() []TranslationEntry {
	entries := r.discoverEntries()
	if len(entries) == 0 {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		aRank := symfonyLocaleRank(a.Locale)
		bRank := symfonyLocaleRank(b.Locale)
		if aRank != bRank {
			return aRank < bRank
		}
		return a.Locale < b.Locale
	})

	uniq := make([]TranslationEntry, 0, len(entries))
	seen := make(map[string]bool)
	for _, entry := range entries {
		dedupKey := entry.Key + "\x00" + entry.Domain
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true
		uniq = append(uniq, entry)
	}
	return uniq
}

func symfonyLocaleRank(locale string) int {
	if locale == "en" {
		return 0
	}
	return 1
}

func (r *TranslationResolver) discoverEntries() []TranslationEntry {
	if r.rootPath == "" {
		return nil
	}

	translationsDir := filepath.Join(r.rootPath, "translations")
	dirEntries, err := os.ReadDir(translationsDir)
	if err != nil {
		return nil
	}

	var entries []TranslationEntry
	for _, de := range dirEntries {
		if de.IsDir() {
			continue
		}
		path := filepath.Join(translationsDir, de.Name())
		name := de.Name()
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".yaml", ".yml":
			entries = append(entries, discoverYAMLTranslationEntries(path)...)
		case ".php":
			entries = append(entries, discoverPHPTranslationEntries(path)...)
		case ".xlf", ".xliff":
			entries = append(entries, discoverXLIFFTranslationEntries(path)...)
		}
	}
	return entries
}

// parseSymfonyTranslationFilename parses a Symfony translation filename into
// domain and locale. Symfony convention: <domain>.<locale>.yaml
// e.g. messages.en.yaml → domain="messages", locale="en"
//
//	validators.fr.xlf  → domain="validators", locale="fr"
func parseSymfonyTranslationFilename(base string) (domain, locale string) {
	// Strip known extensions (including multi-part like .xliff)
	for _, ext := range []string{".xliff", ".xlf", ".yaml", ".yml", ".php"} {
		if strings.HasSuffix(strings.ToLower(base), ext) {
			base = base[:len(base)-len(ext)]
			break
		}
	}
	// Split into parts by "."
	parts := strings.SplitN(base, ".", 3)
	switch len(parts) {
	case 1:
		return parts[0], "en"
	case 2:
		// Could be domain.locale or just domain (unusual)
		// Heuristic: if the second part looks like a locale (2-5 chars) treat as locale
		if len(parts[1]) >= 2 && len(parts[1]) <= 5 {
			return parts[0], parts[1]
		}
		return base, "en"
	default:
		return parts[0], parts[1]
	}
}

// ---- YAML translation files ----

func discoverYAMLTranslationEntries(path string) []TranslationEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	domain, locale := parseSymfonyTranslationFilename(filepath.Base(path))
	uri := translationPathToURI(path)
	src := string(data)
	return parseYAMLTranslationContent(src, domain, locale, filepath.Base(path), uri)
}

// parseYAMLTranslationContent parses a YAML translation file content into entries.
// Nested keys are flattened with dots: user.profile.title
// Returns nil (not error) on malformed content — errors are silently skipped.
func parseYAMLTranslationContent(src, domain, locale, file, uri string) []TranslationEntry {
	lines := strings.Split(src, "\n")
	starts := yamlLineStarts(src)

	type stackEntry struct {
		prefix string
		indent int
	}

	var entries []TranslationEntry
	var stack []stackEntry

	for lineNo, line := range lines {
		// Skip blank lines and comments
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := leadingYAMLIndent(line)

		// Pop stack entries that are deeper or equal to current indent
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}

		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			continue
		}

		key := strings.TrimSpace(trimmed[:colon])
		// Keys with spaces or starting with - are not valid mapping keys
		if strings.Contains(key, " ") || strings.HasPrefix(key, "-") || strings.HasPrefix(key, "'") || strings.HasPrefix(key, "\"") {
			continue
		}

		valueStr := strings.TrimSpace(trimmed[colon+1:])
		// Strip inline comment
		if commentIdx := findYAMLInlineComment(valueStr); commentIdx >= 0 {
			valueStr = strings.TrimSpace(valueStr[:commentIdx])
		}

		// Build full dotted key
		prefix := ""
		if len(stack) > 0 {
			prefix = stack[len(stack)-1].prefix
		}
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		if valueStr == "" || valueStr == "|" || valueStr == ">" || valueStr == "|-" || valueStr == ">-" {
			// Multi-line or nested value — push as prefix and continue
			stack = append(stack, stackEntry{prefix: fullKey, indent: indent})
			continue
		}

		// It's a leaf: compute range and add entry
		// Find the column of the key on this line
		keyCol := indent
		rng := textpos.Range{
			Start: textpos.Position{Line: lineNo, Character: keyCol},
			End:   textpos.Position{Line: lineNo, Character: keyCol + len(key)},
		}
		_ = starts // use offset-based if needed; line-number is simpler here

		entries = append(entries, TranslationEntry{
			Key:    fullKey,
			Domain: domain,
			Locale: locale,
			File:   file,
			URI:    uri,
			Range:  rng,
		})
	}
	return entries
}

// findYAMLInlineComment returns the index of a ` #` inline comment, or -1.
func findYAMLInlineComment(s string) int {
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			inQuote = ch
			continue
		}
		if ch == '#' && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			return i
		}
	}
	return -1
}

func leadingYAMLIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func yamlLineStarts(src string) []int {
	starts := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// ---- PHP array translation files ----

// discoverPHPTranslationEntries parses a PHP array translation file.
// Reuses the same hand-rolled approach as the Laravel resolver.
func discoverPHPTranslationEntries(path string) []TranslationEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	domain, locale := parseSymfonyTranslationFilename(filepath.Base(path))
	uri := translationPathToURI(path)
	src := string(data)

	open := strings.Index(src, "[")
	if open < 0 {
		return nil
	}

	p := &phpSymfonyTranslationParser{
		src:        src,
		path:       path,
		uri:        uri,
		domain:     domain,
		locale:     locale,
		lineStarts: yamlLineStarts(src),
	}
	p.parseArray(open, "")
	return p.entries
}

type phpSymfonyTranslationParser struct {
	src        string
	path       string
	uri        string
	domain     string
	locale     string
	lineStarts []int
	entries    []TranslationEntry
}

func (p *phpSymfonyTranslationParser) parseArray(idx int, prefix string) int {
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

		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		if idx < len(p.src) && p.src[idx] == '[' {
			idx = p.parseArray(idx, fullKey)
		} else {
			p.entries = append(p.entries, TranslationEntry{
				Key:    fullKey,
				Domain: p.domain,
				Locale: p.locale,
				File:   filepath.Base(p.path),
				URI:    p.uri,
				Range:  p.rangeForOffsets(keyStart, keyEnd),
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

func (p *phpSymfonyTranslationParser) parseQuotedString(idx int) (value string, start, end, next int, ok bool) {
	if idx >= len(p.src) || (p.src[idx] != '\'' && p.src[idx] != '"') {
		return "", 0, 0, idx, false
	}
	quote := p.src[idx]
	start = idx + 1
	for i := start; i < len(p.src); i++ {
		if p.src[i] == quote && (i == 0 || p.src[i-1] != '\\') {
			return p.src[start:i], start, i, i + 1, true
		}
	}
	return "", 0, 0, idx, false
}

func (p *phpSymfonyTranslationParser) skipIgnorable(idx int) int {
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

func (p *phpSymfonyTranslationParser) skipValue(idx int) int {
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

func (p *phpSymfonyTranslationParser) rangeForOffsets(start, end int) textpos.Range {
	return textpos.Range{
		Start: xliffOffsetPosition(p.lineStarts, start),
		End:   xliffOffsetPosition(p.lineStarts, end),
	}
}

// ---- XLIFF translation files ----

// xliffTransUnit is the XML structure for a trans-unit element.
type xliffTransUnit struct {
	ID     string `xml:"id,attr"`
	Source string `xml:"source"`
}

// xliffFile wraps an XLIFF file element.
type xliffFileV1 struct {
	Body struct {
		TransUnits []xliffTransUnit `xml:"trans-unit"`
	} `xml:"body"`
}

type xliffDocV1 struct {
	XMLName xml.Name      `xml:"xliff"`
	File    []xliffFileV1 `xml:"file"`
}

// xliff2Unit is the XLIFF 2.x unit element.
type xliff2Unit struct {
	ID      string `xml:"id,attr"`
	Segment struct {
		Source string `xml:"source"`
	} `xml:"segment"`
}

type xliff2File struct {
	Units []xliff2Unit `xml:"unit"`
}

type xliffDocV2 struct {
	XMLName xml.Name     `xml:"xliff"`
	File    []xliff2File `xml:"file"`
}

func discoverXLIFFTranslationEntries(path string) []TranslationEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	domain, locale := parseSymfonyTranslationFilename(filepath.Base(path))
	uri := translationPathToURI(path)
	src := string(data)

	entries := parseXLIFFContent(src, domain, locale, filepath.Base(path), uri)
	return entries
}

// parseXLIFFContent parses XLIFF 1.x and 2.x content. Malformed XML is silently
// skipped (returns empty slice).
func parseXLIFFContent(src, domain, locale, file, uri string) []TranslationEntry {
	// Detect version from xliff element
	var entries []TranslationEntry

	// Try XLIFF 1.x first
	var docV1 xliffDocV1
	if err := xml.Unmarshal([]byte(src), &docV1); err == nil && len(docV1.File) > 0 {
		for _, f := range docV1.File {
			for _, unit := range f.Body.TransUnits {
				if unit.ID == "" && unit.Source == "" {
					continue
				}
				key := unit.ID
				if key == "" {
					key = unit.Source
				}
				rng := findXLIFFKeyRange(src, unit.ID)
				entries = append(entries, TranslationEntry{
					Key:    key,
					Domain: domain,
					Locale: locale,
					File:   file,
					URI:    uri,
					Range:  rng,
				})
			}
		}
		if len(entries) > 0 {
			return entries
		}
	}

	// Try XLIFF 2.x
	var docV2 xliffDocV2
	if err := xml.Unmarshal([]byte(src), &docV2); err == nil {
		for _, f := range docV2.File {
			for _, unit := range f.Units {
				if unit.ID == "" {
					continue
				}
				key := unit.ID
				if unit.Segment.Source != "" {
					key = unit.Segment.Source
				}
				rng := findXLIFFKeyRange(src, unit.ID)
				entries = append(entries, TranslationEntry{
					Key:    key,
					Domain: domain,
					Locale: locale,
					File:   file,
					URI:    uri,
					Range:  rng,
				})
			}
		}
	}

	return entries
}

// findXLIFFKeyRange finds the source element text range within the XLIFF source,
// searching for the id attribute value or the source element content.
func findXLIFFKeyRange(src, id string) textpos.Range {
	if id == "" {
		return textpos.Range{}
	}
	needle := `id="` + id + `"`
	offset := strings.Index(src, needle)
	if offset < 0 {
		return textpos.Range{}
	}
	valueOffset := offset + 4 // after id="
	starts := yamlLineStarts(src)
	start := xliffOffsetPosition(starts, valueOffset)
	end := xliffOffsetPosition(starts, valueOffset+len(id))
	return textpos.Range{Start: start, End: end}
}

func xliffOffsetPosition(starts []int, offset int) textpos.Position {
	line := sort.Search(len(starts), func(i int) bool { return starts[i] > offset }) - 1
	if line < 0 {
		line = 0
	}
	return textpos.Position{
		Line:      line,
		Character: offset - starts[line],
	}
}

func translationPathToURI(path string) string {
	abs := path
	if a, err := filepath.Abs(path); err == nil {
		abs = a
	}
	return symbols.PathToURI(abs)
}
