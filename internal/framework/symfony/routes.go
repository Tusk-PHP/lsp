package symfony

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

type Route struct {
	Name      string
	Path      string
	URI       string
	NameRange protocol.Range
	DeclRange protocol.Range
}

type routeAttribute struct {
	Name      string
	Path      string
	NameRange protocol.Range
	AttrRange protocol.Range
}

var routeNamePattern = regexp.MustCompile(`name\s*:\s*['"]([^'"]*)['"]`)
var routePathPattern = regexp.MustCompile(`path\s*:\s*['"]([^'"]*)['"]`)
var firstStringPattern = regexp.MustCompile(`^\s*['"]([^'"]*)['"]`)

type routeConfigEntry struct {
	Name       string
	Path       string
	URI        string
	DeclRange  protocol.Range
	IsImport   bool
	Resource   string
	Type       string
	Prefix     string
	NamePrefix string
}

type xmlRoutes struct {
	XMLName xml.Name    `xml:"routes"`
	Routes  []xmlRoute  `xml:"route"`
	Imports []xmlImport `xml:"import"`
}

type xmlRoute struct {
	ID   string `xml:"id,attr"`
	Path string `xml:"path,attr"`
}

type xmlImport struct {
	Resource   string `xml:"resource,attr"`
	Type       string `xml:"type,attr"`
	Prefix     string `xml:"prefix,attr"`
	NamePrefix string `xml:"name-prefix,attr"`
}

// DiscoverRoutes scans indexed project PHP sources and Symfony route config
// files, then returns named routes discovered from both sources.
func DiscoverRoutes(index *symbols.Index) []Route {
	if index == nil {
		return nil
	}

	merged := make(map[string]Route)
	for _, route := range discoverAttributeRoutes(index) {
		if route.Name == "" {
			continue
		}
		merged[route.Name] = route
	}
	for _, route := range discoverConfigRoutes(index) {
		if route.Name == "" {
			continue
		}
		merged[route.Name] = route
	}

	routes := make([]Route, 0, len(merged))
	for _, route := range merged {
		routes = append(routes, route)
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Name != routes[j].Name {
			return routes[i].Name < routes[j].Name
		}
		return routes[i].URI < routes[j].URI
	})
	return routes
}

func discoverAttributeRoutes(index *symbols.Index) []Route {
	var routes []Route
	for _, uri := range index.GetAllFileURIs() {
		if strings.Contains(uri, "/vendor/") {
			continue
		}
		source := index.GetFileSource(uri)
		if source == "" || !strings.Contains(source, "Route") || !strings.Contains(source, "#[") {
			continue
		}
		routes = append(routes, discoverRoutesInFile(uri, source)...)
	}
	return routes
}

func discoverConfigRoutes(index *symbols.Index) []Route {
	var routes []Route
	visited := make(map[string]bool)

	for _, root := range discoverSymfonyRoots(index) {
		configDir := filepath.Join(root, "config")
		for _, name := range []string{"routes.yaml", "routes.yml", "routes.xml"} {
			path := filepath.Join(configDir, name)
			if _, err := os.Stat(path); err != nil {
				continue
			}
			routes = append(routes, discoverRoutesFromConfigPath(path, "", "", visited)...)
		}
	}

	return routes
}

func discoverSymfonyRoots(index *symbols.Index) []string {
	seen := make(map[string]bool)
	var roots []string

	for _, uri := range index.GetAllFileURIs() {
		if strings.Contains(uri, "/vendor/") {
			continue
		}
		path := symbols.URIToPath(uri)
		if path == "" {
			continue
		}

		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		dir := filepath.Dir(path)
		for {
			if dir == "." || dir == string(filepath.Separator) || dir == "" {
				break
			}
			if _, err := os.Stat(filepath.Join(dir, "composer.json")); err == nil {
				if !seen[dir] {
					seen[dir] = true
					roots = append(roots, dir)
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	sort.Strings(roots)
	return roots
}

func discoverRoutesFromConfigPath(path, pathPrefix, namePrefix string, visited map[string]bool) []Route {
	cleanPath := filepath.Clean(path)
	if visited[cleanPath] {
		return nil
	}
	visited[cleanPath] = true

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil
	}

	var entries []routeConfigEntry
	switch strings.ToLower(filepath.Ext(cleanPath)) {
	case ".yaml", ".yml":
		entries = parseYAMLRouteConfig(cleanPath, string(content))
	case ".xml":
		entries = parseXMLRouteConfig(cleanPath, string(content))
	default:
		return nil
	}

	var routes []Route
	baseDir := filepath.Dir(cleanPath)
	for _, entry := range entries {
		entryPathPrefix := joinRoutePath(pathPrefix, entry.Prefix)
		entryNamePrefix := namePrefix + entry.NamePrefix

		if entry.IsImport {
			resourcePath := filepath.Clean(filepath.Join(baseDir, entry.Resource))
			routes = append(routes, discoverRoutesFromResource(resourcePath, entry.Type, entryPathPrefix, entryNamePrefix, visited)...)
			continue
		}

		routes = append(routes, Route{
			Name:      entryNamePrefix + entry.Name,
			Path:      joinRoutePath(entryPathPrefix, entry.Path),
			URI:       entry.URI,
			NameRange: entry.DeclRange,
			DeclRange: entry.DeclRange,
		})
	}

	return routes
}

func discoverRoutesFromResource(path, resourceType, pathPrefix, namePrefix string, visited map[string]bool) []Route {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	if info.IsDir() {
		if resourceType != "attribute" && resourceType != "" {
			return nil
		}
		return discoverRoutesFromAttributeDir(path, pathPrefix, namePrefix)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml", ".xml":
		return discoverRoutesFromConfigPath(path, pathPrefix, namePrefix, visited)
	case ".php":
		if resourceType != "attribute" && resourceType != "" {
			return nil
		}
		return discoverRoutesFromAttributeFile(path, pathPrefix, namePrefix)
	default:
		return nil
	}
}

func discoverRoutesFromAttributeDir(root, pathPrefix, namePrefix string) []Route {
	var routes []Route
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		routes = append(routes, discoverRoutesFromAttributeFile(path, pathPrefix, namePrefix)...)
		return nil
	})
	return routes
}

func discoverRoutesFromAttributeFile(path, pathPrefix, namePrefix string) []Route {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	uri := path
	if abs, err := filepath.Abs(path); err == nil {
		uri = "file://" + filepath.ToSlash(abs)
	}

	routes := discoverRoutesInFile(uri, string(source))
	for i := range routes {
		routes[i].Name = namePrefix + routes[i].Name
		routes[i].Path = joinRoutePath(pathPrefix, routes[i].Path)
	}
	return routes
}

func FindRoute(index *symbols.Index, name string) *Route {
	for _, route := range DiscoverRoutes(index) {
		if route.Name == name {
			route := route
			return &route
		}
	}
	return nil
}

func ExtractRouteNameContext(trimmed string) (partial, quote string, ok bool) {
	patterns := []string{
		"generateUrl(",
		"redirectToRoute(",
		"->generate(",
	}
	for _, pat := range patterns {
		idx := strings.LastIndex(trimmed, pat)
		if idx < 0 {
			continue
		}
		after := trimmed[idx+len(pat):]
		if strings.Contains(after, ")") || strings.Contains(after, ",") {
			continue
		}
		after = strings.TrimLeft(after, " \t")
		if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
			return after[1:], string(after[0]), true
		}
		return after, "", true
	}
	return "", "", false
}

func RouteNameAtPosition(source string, pos protocol.Position) (string, protocol.Range, bool) {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return "", protocol.Range{}, false
	}
	line := lines[pos.Line]
	if pos.Character < 0 || pos.Character > len(line) {
		return "", protocol.Range{}, false
	}

	patterns := []string{
		"generateUrl(",
		"redirectToRoute(",
		"->generate(",
	}
	for _, pat := range patterns {
		idx := strings.LastIndex(line[:pos.Character], pat)
		if idx < 0 {
			continue
		}
		i := idx + len(pat)
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) || (line[i] != '\'' && line[i] != '"') {
			continue
		}
		quote := line[i]
		start := i + 1
		end := start
		for end < len(line) {
			if line[end] == '\\' {
				end += 2
				continue
			}
			if line[end] == quote {
				break
			}
			end++
		}
		if end >= len(line) || pos.Character < start || pos.Character > end {
			continue
		}
		if comma := strings.Index(line[idx+len(pat):start-1], ","); comma >= 0 {
			continue
		}
		return line[start:end], protocol.Range{
			Start: protocol.Position{Line: pos.Line, Character: start},
			End:   protocol.Position{Line: pos.Line, Character: end},
		}, true
	}
	return "", protocol.Range{}, false
}

func discoverRoutesInFile(uri, source string) []Route {
	file := parser.ParseFile(source)
	if file == nil {
		return nil
	}

	lines := strings.Split(source, "\n")
	var routes []Route

	for _, class := range file.Classes {
		classAttrs := routeAttributesNearLine(lines, class.StartLine)
		classPrefix := mergeRoutePrefix(classAttrs)

		for _, method := range class.Methods {
			methodAttrs := routeAttributesNearLine(lines, method.StartLine)
			for _, attr := range methodAttrs {
				name := attr.Name
				if name == "" {
					continue
				}
				if classPrefix.Name != "" {
					name = classPrefix.Name + name
				}

				declRange := attr.AttrRange
				if attr.NameRange != (protocol.Range{}) {
					declRange = attr.NameRange
				}

				routes = append(routes, Route{
					Name:      name,
					Path:      classPrefix.Path + attr.Path,
					URI:       uri,
					NameRange: attr.NameRange,
					DeclRange: declRange,
				})
			}
		}
	}

	return routes
}

func mergeRoutePrefix(attrs []routeAttribute) routeAttribute {
	for _, attr := range attrs {
		if attr.Name != "" || attr.Path != "" {
			return attr
		}
	}
	return routeAttribute{}
}

func routeAttributesNearLine(lines []string, line int) []routeAttribute {
	if line < 0 || line >= len(lines) {
		return nil
	}

	declLine := line
	for declLine < len(lines) && !looksLikeDeclarationLine(lines[declLine]) {
		declLine++
	}
	if declLine >= len(lines) {
		return nil
	}

	start := declLine - 1
	for start >= 0 {
		trimmed := strings.TrimSpace(lines[start])
		if trimmed == "" || trimmed == "{" || trimmed == "}" || looksLikeDeclarationLine(lines[start]) {
			break
		}
		start--
	}
	start++

	snippet := strings.Join(lines[start:declLine+1], "\n")
	if !strings.Contains(snippet, "#[") {
		return nil
	}

	return parseRouteAttributes(snippet, start)
}

func looksLikeDeclarationLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.Contains(trimmed, "class ") ||
		strings.Contains(trimmed, "interface ") ||
		strings.Contains(trimmed, "trait ") ||
		strings.Contains(trimmed, "enum ") ||
		strings.Contains(trimmed, "function ")
}

func parseRouteAttributes(snippet string, baseLine int) []routeAttribute {
	var attrs []routeAttribute
	for i := 0; i < len(snippet); i++ {
		if i+1 >= len(snippet) || snippet[i] != '#' || snippet[i+1] != '[' {
			continue
		}
		end := findAttributeEnd(snippet, i+2)
		if end < 0 {
			continue
		}

		block := snippet[i : end+1]
		attr, ok := parseRouteAttributeBlock(snippet, block, baseLine, i)
		if ok {
			attrs = append(attrs, attr)
		}
		i = end
	}
	return attrs
}

func findAttributeEnd(snippet string, start int) int {
	inString := byte(0)
	parenDepth := 0
	for i := start; i < len(snippet); i++ {
		ch := snippet[i]
		if inString != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == inString {
				inString = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			inString = ch
			continue
		}
		if ch == '(' {
			parenDepth++
			continue
		}
		if ch == ')' && parenDepth > 0 {
			parenDepth--
			continue
		}
		if ch == ']' && parenDepth == 0 {
			return i
		}
	}
	return -1
}

func parseRouteAttributeBlock(snippet, block string, baseLine, blockOffset int) (routeAttribute, bool) {
	open := strings.Index(block, "(")
	close := strings.LastIndex(block, ")")
	if open < 0 || close <= open {
		return routeAttribute{}, false
	}

	namePart := strings.TrimSpace(block[2:open])
	parts := strings.Split(namePart, "\\")
	short := parts[len(parts)-1]
	if short != "Route" {
		return routeAttribute{}, false
	}

	args := block[open+1 : close]
	attr := routeAttribute{
		AttrRange: protocol.Range{
			Start: positionFromOffset(snippet, baseLine, blockOffset),
			End:   positionFromOffset(snippet, baseLine, blockOffset+len(block)),
		},
	}

	if match := routeNamePattern.FindStringSubmatchIndex(args); len(match) >= 4 {
		attr.Name = args[match[2]:match[3]]
		attr.NameRange = protocol.Range{
			Start: positionFromOffset(snippet, baseLine, blockOffset+open+1+match[2]),
			End:   positionFromOffset(snippet, baseLine, blockOffset+open+1+match[3]),
		}
	}
	if match := routePathPattern.FindStringSubmatchIndex(args); len(match) >= 4 {
		attr.Path = args[match[2]:match[3]]
	} else if match := firstStringPattern.FindStringSubmatchIndex(args); len(match) >= 4 {
		attr.Path = args[match[2]:match[3]]
	}

	return attr, true
}

func positionFromOffset(text string, baseLine, offset int) protocol.Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}

	line := baseLine
	col := 0
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line++
			col = 0
			continue
		}
		col++
	}
	return protocol.Position{Line: line, Character: col}
}

func parseYAMLRouteConfig(uri, source string) []routeConfigEntry {
	lines := strings.Split(source, "\n")
	var entries []routeConfigEntry

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || leadingIndent(line) != 0 {
			continue
		}

		colon := strings.Index(trimmed, ":")
		if colon <= 0 {
			continue
		}

		name := strings.TrimSpace(trimmed[:colon])
		if strings.Contains(name, " ") || strings.HasPrefix(name, "-") {
			continue
		}

		entry := routeConfigEntry{
			Name:      name,
			URI:       "file://" + filepath.ToSlash(uri),
			DeclRange: protocol.Range{Start: protocol.Position{Line: i, Character: 0}, End: protocol.Position{Line: i, Character: len(name)}},
		}

		baseIndent := leadingIndent(line)
		for j := i + 1; j < len(lines); j++ {
			childLine := lines[j]
			childTrimmed := strings.TrimSpace(childLine)
			if childTrimmed == "" || strings.HasPrefix(childTrimmed, "#") {
				continue
			}
			childIndent := leadingIndent(childLine)
			if childIndent <= baseIndent {
				break
			}

			key, value, ok := splitYAMLKeyValue(childTrimmed)
			if !ok {
				continue
			}

			switch key {
			case "path":
				entry.Path = trimYAMLScalar(value)
			case "resource":
				if value != "" {
					entry.IsImport = true
					entry.Resource = trimYAMLScalar(value)
					continue
				}
				for k := j + 1; k < len(lines); k++ {
					nestedLine := lines[k]
					nestedTrimmed := strings.TrimSpace(nestedLine)
					if nestedTrimmed == "" || strings.HasPrefix(nestedTrimmed, "#") {
						continue
					}
					nestedIndent := leadingIndent(nestedLine)
					if nestedIndent <= childIndent {
						break
					}
					nestedKey, nestedValue, ok := splitYAMLKeyValue(nestedTrimmed)
					if !ok {
						continue
					}
					if nestedKey == "path" {
						entry.IsImport = true
						entry.Resource = trimYAMLScalar(nestedValue)
					}
				}
			case "type":
				entry.Type = trimYAMLScalar(value)
			case "prefix":
				entry.Prefix = trimYAMLScalar(value)
			case "name_prefix":
				entry.NamePrefix = trimYAMLScalar(value)
			}
		}

		if entry.IsImport || entry.Path != "" {
			entries = append(entries, entry)
		}
	}

	return entries
}

func parseXMLRouteConfig(uri, source string) []routeConfigEntry {
	var doc xmlRoutes
	if err := xml.Unmarshal([]byte(source), &doc); err != nil {
		return nil
	}

	var entries []routeConfigEntry
	for _, route := range doc.Routes {
		if route.ID == "" {
			continue
		}
		rng := findXMLDeclRange(source, "id", route.ID)
		entries = append(entries, routeConfigEntry{
			Name:      route.ID,
			Path:      route.Path,
			URI:       "file://" + filepath.ToSlash(uri),
			DeclRange: rng,
		})
	}
	for _, imp := range doc.Imports {
		if imp.Resource == "" {
			continue
		}
		entries = append(entries, routeConfigEntry{
			URI:        "file://" + filepath.ToSlash(uri),
			DeclRange:  findXMLDeclRange(source, "resource", imp.Resource),
			IsImport:   true,
			Resource:   imp.Resource,
			Type:       imp.Type,
			Prefix:     imp.Prefix,
			NamePrefix: imp.NamePrefix,
		})
	}
	return entries
}

func joinRoutePath(prefix, path string) string {
	switch {
	case prefix == "":
		return path
	case path == "":
		return prefix
	case strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, "/"):
		return prefix + strings.TrimPrefix(path, "/")
	case !strings.HasSuffix(prefix, "/") && !strings.HasPrefix(path, "/"):
		return prefix + "/" + path
	default:
		return prefix + path
	}
}

func leadingIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func splitYAMLKeyValue(line string) (string, string, bool) {
	colon := strings.Index(line, ":")
	if colon <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:colon]), strings.TrimSpace(line[colon+1:]), true
}

func trimYAMLScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, " #"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, `'"`)
}

func findXMLDeclRange(source, attr, value string) protocol.Range {
	needle := attr + `="` + value + `"`
	offset := strings.Index(source, needle)
	if offset < 0 {
		return protocol.Range{}
	}

	valueOffset := offset + len(attr) + 2
	start := positionFromOffset(source, 0, valueOffset)
	end := positionFromOffset(source, 0, valueOffset+len(value))
	return protocol.Range{Start: start, End: end}
}
