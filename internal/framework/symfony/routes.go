package symfony

import (
	"regexp"
	"sort"
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

// DiscoverRoutes scans indexed project PHP sources for Symfony #[Route(...)]
// attributes and returns named routes discovered from controller classes.
func DiscoverRoutes(index *symbols.Index) []Route {
	if index == nil {
		return nil
	}

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

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Name != routes[j].Name {
			return routes[i].Name < routes[j].Name
		}
		return routes[i].URI < routes[j].URI
	})
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
