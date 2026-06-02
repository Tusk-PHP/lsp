package laravel

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type RouteName struct {
	Name          string
	Path          string
	URI           string
	Range         protocol.Range
	TargetKind    string
	Target        string
	HandlerFQN    string
	HandlerMethod string
}

type RouteIndex struct {
	rootPath string

	mu     sync.RWMutex
	byURI  map[string][]RouteName
	byName map[string][]RouteName
}

func NewRouteIndex(rootPath string) *RouteIndex {
	return &RouteIndex{
		rootPath: rootPath,
		byURI:    make(map[string][]RouteName),
		byName:   make(map[string][]RouteName),
	}
}

func (idx *RouteIndex) ScanWorkspace() error {
	routesDir := filepath.Join(idx.rootPath, "routes")
	info, err := os.Stat(routesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.Walk(routesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		idx.IndexFile("file://"+path, string(content))
		return nil
	})
}

func (idx *RouteIndex) IndexFile(uri, source string) {
	if !isLaravelRouteFile(uri) {
		return
	}

	routes := parseRouteDefinitions(uri, source)

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if prev := idx.byURI[uri]; len(prev) > 0 {
		for _, route := range prev {
			idx.removeRouteLocked(route)
		}
	}

	idx.byURI[uri] = routes
	for _, route := range routes {
		idx.byName[route.Name] = append(idx.byName[route.Name], route)
		idx.sortRouteDefsLocked(route.Name)
	}
}

func (idx *RouteIndex) Names() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	names := make([]string, 0, len(idx.byName))
	for name, entries := range idx.byName {
		if name != "" && len(entries) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (idx *RouteIndex) Find(name string) *RouteName {
	if idx == nil || name == "" {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	entries := idx.byName[name]
	if len(entries) == 0 {
		return nil
	}
	route := entries[0]
	return &route
}

func (idx *RouteIndex) All() []RouteName {
	if idx == nil {
		return nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var routes []RouteName
	for _, entries := range idx.byName {
		routes = append(routes, entries...)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Name != routes[j].Name {
			return routes[i].Name < routes[j].Name
		}
		if routes[i].URI != routes[j].URI {
			return routes[i].URI < routes[j].URI
		}
		if routes[i].Range.Start.Line != routes[j].Range.Start.Line {
			return routes[i].Range.Start.Line < routes[j].Range.Start.Line
		}
		return routes[i].Range.Start.Character < routes[j].Range.Start.Character
	})
	return routes
}

func (idx *RouteIndex) removeRouteLocked(route RouteName) {
	entries := idx.byName[route.Name]
	if len(entries) == 0 {
		return
	}

	filtered := entries[:0]
	for _, entry := range entries {
		if entry.URI == route.URI &&
			entry.Range.Start == route.Range.Start &&
			entry.Range.End == route.Range.End {
			continue
		}
		filtered = append(filtered, entry)
	}

	if len(filtered) == 0 {
		delete(idx.byName, route.Name)
		return
	}
	idx.byName[route.Name] = filtered
}

func (idx *RouteIndex) sortRouteDefsLocked(name string) {
	sort.Slice(idx.byName[name], func(i, j int) bool {
		a := idx.byName[name][i]
		b := idx.byName[name][j]
		if a.URI != b.URI {
			return a.URI < b.URI
		}
		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line < b.Range.Start.Line
		}
		return a.Range.Start.Character < b.Range.Start.Character
	})
}

var supportedRouteMethods = map[string]bool{
	"get":      true,
	"post":     true,
	"put":      true,
	"patch":    true,
	"delete":   true,
	"options":  true,
	"match":    true,
	"any":      true,
	"view":     true,
	"livewire": true,
	"redirect": true,
}

func parseRouteDefinitions(uri, source string) []RouteName {
	result := parser.New().Parse(source)
	file := parser.ParseFile(source)
	var routes []RouteName

	for i := 0; i < len(result.Tokens); i++ {
		if result.Tokens[i].Kind != parser.TokenIdentifier || result.Tokens[i].Value != "Route" {
			continue
		}
		doubleColon := nextSignificantToken(result.Tokens, i+1)
		if doubleColon < 0 || result.Tokens[doubleColon].Kind != parser.TokenDoubleColon {
			continue
		}
		methodTok := nextSignificantToken(result.Tokens, doubleColon+1)
		if methodTok < 0 || result.Tokens[methodTok].Kind != parser.TokenIdentifier {
			continue
		}
		method := result.Tokens[methodTok].Value
		if !supportedRouteMethods[method] {
			continue
		}
		route, _, ok := parseRouteDefinition(result.Tokens, uri, methodTok, method, file)
		if !ok {
			continue
		}
		routes = append(routes, route)
	}

	return routes
}

func parseRouteDefinition(tokens []parser.Token, uri string, methodTok int, method string, file *parser.FileNode) (RouteName, int, bool) {
	openTok := nextSignificantToken(tokens, methodTok+1)
	if openTok < 0 || tokens[openTok].Kind != parser.TokenOpenParen {
		return RouteName{}, -1, false
	}
	closeTok := findMatchingToken(tokens, openTok, parser.TokenOpenParen, parser.TokenCloseParen)
	if closeTok < 0 {
		return RouteName{}, -1, false
	}

	args := splitTopLevelArgs(tokens, openTok+1, closeTok)
	route := RouteName{URI: uri}
	if len(args) > 0 {
		if path, ok := firstStringLiteral(args[0]); ok {
			route.Path = path
		}
	}
	route.TargetKind, route.Target, route.HandlerFQN, route.HandlerMethod = parseRouteTarget(method, args, file)

	end := closeTok
	for i := closeTok + 1; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenSemicolon:
			end = i
			return route, end, route.Name != ""
		case parser.TokenArrow:
			nameTok := nextSignificantToken(tokens, i+1)
			if nameTok < 0 || tokens[nameTok].Kind != parser.TokenIdentifier {
				continue
			}
			if tokens[nameTok].Value != "name" {
				continue
			}
			nameOpen := nextSignificantToken(tokens, nameTok+1)
			if nameOpen < 0 || tokens[nameOpen].Kind != parser.TokenOpenParen {
				continue
			}
			valueTok := nextSignificantToken(tokens, nameOpen+1)
			if valueTok < 0 || tokens[valueTok].Kind != parser.TokenStringLiteral {
				continue
			}
			raw := tokens[valueTok].Value
			name := trimQuotedLiteral(raw)
			if name == "" {
				continue
			}
			startCol := tokens[valueTok].Column
			endCol := startCol + len(raw)
			if len(raw) >= 2 {
				startCol++
				endCol--
			}
			route.Name = name
			route.Range = protocol.Range{
				Start: protocol.Position{Line: tokens[valueTok].Line, Character: startCol},
				End:   protocol.Position{Line: tokens[valueTok].Line, Character: endCol},
			}
		}
	}

	return route, end, route.Name != ""
}

func nextSignificantToken(tokens []parser.Token, start int) int {
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			continue
		default:
			return i
		}
	}
	return -1
}

func trimQuotedLiteral(raw string) string {
	if len(raw) >= 2 && (raw[0] == '\'' || raw[0] == '"') && raw[len(raw)-1] == raw[0] {
		return raw[1 : len(raw)-1]
	}
	return raw
}

func findMatchingToken(tokens []parser.Token, openIdx int, openKind, closeKind parser.TokenKind) int {
	depth := 0
	for i := openIdx; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case openKind:
			depth++
		case closeKind:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevelArgs(tokens []parser.Token, start, end int) [][]parser.Token {
	var args [][]parser.Token
	argStart := start
	parenDepth, bracketDepth, braceDepth := 0, 0, 0
	for i := start; i < end; i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen:
			parenDepth++
		case parser.TokenCloseParen:
			parenDepth--
		case parser.TokenOpenBracket:
			bracketDepth++
		case parser.TokenCloseBracket:
			bracketDepth--
		case parser.TokenOpenBrace:
			braceDepth++
		case parser.TokenCloseBrace:
			braceDepth--
		case parser.TokenComma:
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				args = append(args, trimInsignificantTokens(tokens[argStart:i]))
				argStart = i + 1
			}
		}
	}
	if argStart <= end {
		args = append(args, trimInsignificantTokens(tokens[argStart:end]))
	}
	return args
}

func trimInsignificantTokens(tokens []parser.Token) []parser.Token {
	start := 0
	for start < len(tokens) && isInsignificant(tokens[start].Kind) {
		start++
	}
	end := len(tokens)
	for end > start && isInsignificant(tokens[end-1].Kind) {
		end--
	}
	return tokens[start:end]
}

func isInsignificant(kind parser.TokenKind) bool {
	return kind == parser.TokenWhitespace || kind == parser.TokenComment || kind == parser.TokenDocComment
}

func firstStringLiteral(tokens []parser.Token) (string, bool) {
	tokens = trimInsignificantTokens(tokens)
	if len(tokens) == 0 || tokens[0].Kind != parser.TokenStringLiteral {
		return "", false
	}
	return trimQuotedLiteral(tokens[0].Value), true
}

func parseRouteTarget(method string, args [][]parser.Token, file *parser.FileNode) (kind, target, handlerFQN, handlerMethod string) {
	switch method {
	case "view":
		if len(args) > 1 {
			if value, ok := firstStringLiteral(args[1]); ok {
				return "view", value, "", ""
			}
		}
		return "view", "", "", ""
	case "livewire":
		if len(args) > 1 {
			if value, ok := firstStringLiteral(args[1]); ok {
				return "livewire", value, "", ""
			}
		}
		return "livewire", "", "", ""
	case "redirect":
		if len(args) > 1 {
			if value, ok := firstStringLiteral(args[1]); ok {
				return "redirect", value, "", ""
			}
		}
		return "redirect", "", "", ""
	default:
		if len(args) < 2 {
			return "route", "", "", ""
		}
		action := trimInsignificantTokens(args[1])
		if className, methodName, ok := parseControllerArrayAction(action, file); ok {
			return "controller", className + "@" + methodName, className, methodName
		}
		if value, ok := firstStringLiteral(action); ok {
			if strings.Contains(value, "@") {
				parts := strings.SplitN(value, "@", 2)
				className := resolveRouteClassName(parts[0], file)
				methodName := parts[1]
				return "controller", className + "@" + methodName, className, methodName
			}
			return "string", value, "", ""
		}
		if className, ok := parseClassConstExpr(action, file); ok {
			return "controller", className + "@__invoke", className, "__invoke"
		}
		return "route", tokenText(action), "", ""
	}
}

func parseControllerArrayAction(tokens []parser.Token, file *parser.FileNode) (className, methodName string, ok bool) {
	tokens = trimInsignificantTokens(tokens)
	if len(tokens) < 5 || tokens[0].Kind != parser.TokenOpenBracket || tokens[len(tokens)-1].Kind != parser.TokenCloseBracket {
		return "", "", false
	}
	body := trimInsignificantTokens(tokens[1 : len(tokens)-1])
	commaIdx := -1
	depth := 0
	for i, tok := range body {
		switch tok.Kind {
		case parser.TokenOpenBracket, parser.TokenOpenParen, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseBracket, parser.TokenCloseParen, parser.TokenCloseBrace:
			depth--
		case parser.TokenComma:
			if depth == 0 {
				commaIdx = i
				break
			}
		}
	}
	if commaIdx < 0 {
		return "", "", false
	}
	classTokens := trimInsignificantTokens(body[:commaIdx])
	methodTokens := trimInsignificantTokens(body[commaIdx+1:])
	className, ok = parseClassConstExpr(classTokens, file)
	if !ok {
		return "", "", false
	}
	methodName, ok = firstStringLiteral(methodTokens)
	if !ok || methodName == "" {
		return "", "", false
	}
	return className, methodName, true
}

func parseClassConstExpr(tokens []parser.Token, file *parser.FileNode) (string, bool) {
	tokens = trimInsignificantTokens(tokens)
	if len(tokens) < 3 || tokens[len(tokens)-2].Kind != parser.TokenDoubleColon || tokens[len(tokens)-1].Kind != parser.TokenClass {
		return "", false
	}
	classRef := tokenText(tokens[:len(tokens)-2])
	if classRef == "" {
		return "", false
	}
	return resolveRouteClassName(classRef, file), true
}

func tokenText(tokens []parser.Token) string {
	var b strings.Builder
	for _, tok := range tokens {
		if isInsignificant(tok.Kind) {
			continue
		}
		b.WriteString(tok.Value)
	}
	return b.String()
}

func resolveRouteClassName(name string, file *parser.FileNode) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, `\`)
	if name == "" {
		return ""
	}
	if strings.Contains(name, `\`) {
		return name
	}
	if file != nil {
		for _, use := range file.Uses {
			if use.Alias == name {
				return use.FullName
			}
			if last := filepath.Base(strings.ReplaceAll(use.FullName, `\`, `/`)); last == name {
				return use.FullName
			}
		}
		if file.Namespace != "" {
			return file.Namespace + `\` + name
		}
	}
	return name
}

func isLaravelRouteFile(uri string) bool {
	path := filepath.ToSlash(symbols.URIToPath(uri))
	return strings.HasSuffix(path, ".php") && strings.Contains(path, "/routes/")
}

var routeCallPatterns = []string{
	"route(",
	"to_route(",
	"->route(",
	"::route(",
	"->routeIs(",
}

func ExtractRouteNameArgContext(trimmed string) (string, string, bool) {
	bestIdx := -1
	bestPattern := ""
	for _, pattern := range routeCallPatterns {
		idx := strings.LastIndex(trimmed, pattern)
		if idx > bestIdx {
			bestIdx = idx
			bestPattern = pattern
		}
	}
	if bestIdx < 0 {
		return "", "", false
	}

	after := trimmed[bestIdx+len(bestPattern):]
	if strings.Contains(after, ")") {
		return "", "", false
	}

	quote := ""
	if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
		quote = string(after[0])
		after = after[1:]
	}

	return after, quote, true
}

func FindRouteNameReference(line string, character int) (string, bool) {
	if character < 0 || len(line) == 0 {
		return "", false
	}
	if character >= len(line) {
		character = len(line) - 1
	}
	if character < 0 {
		return "", false
	}

	prefix := strings.TrimSpace(line[:character+1])
	if _, _, ok := ExtractRouteNameArgContext(prefix); !ok {
		return "", false
	}

	start, end, ok := enclosingQuotedString(line, character)
	if !ok {
		return "", false
	}

	if _, _, ok := ExtractRouteNameArgContext(strings.TrimSpace(line[:start+1])); !ok {
		return "", false
	}

	if end-start < 2 {
		return "", false
	}
	return line[start+1 : end], true
}

func enclosingQuotedString(line string, character int) (int, int, bool) {
	for start := character; start >= 0; start-- {
		if !isQuote(line[start]) || isEscaped(line, start) {
			continue
		}
		quote := line[start]
		for end := start + 1; end < len(line); end++ {
			if line[end] != quote || isEscaped(line, end) {
				continue
			}
			if character > start && character < end {
				return start, end, true
			}
			break
		}
	}
	return 0, 0, false
}

func isQuote(ch byte) bool {
	return ch == '\'' || ch == '"'
}

func isEscaped(s string, idx int) bool {
	backslashes := 0
	for i := idx - 1; i >= 0 && s[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}
