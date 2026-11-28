package symfony

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

type Route struct {
	Name      string
	Path      string
	URI       string
	NameRange textpos.Range
	DeclRange textpos.Range
}

type routeAttribute struct {
	Name      string
	Path      string
	NameRange textpos.Range
	AttrRange textpos.Range
}

var routeAttributeFQNs = []string{`Symfony\Component\Routing\Attribute\Route`, `Symfony\Component\Routing\Annotation\Route`}

type routeConfigEntry struct {
	Name       string
	Path       string
	URI        string
	DeclRange  textpos.Range
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
		routes = append(routes, discoverRoutesInFile(uri, source, index.GrammarOf(uri))...)
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
			routes = append(routes, discoverRoutesFromConfigPath(path, "", "", visited, index.Grammars())...)
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

func discoverRoutesFromConfigPath(path, pathPrefix, namePrefix string, visited map[string]bool, grammars phpparser.GrammarRange) []Route {
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
			routes = append(routes, discoverRoutesFromResource(resourcePath, entry.Type, entryPathPrefix, entryNamePrefix, visited, grammars)...)
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

func discoverRoutesFromResource(path, resourceType, pathPrefix, namePrefix string, visited map[string]bool, grammars phpparser.GrammarRange) []Route {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	if info.IsDir() {
		if resourceType != "attribute" && resourceType != "" {
			return nil
		}
		return discoverRoutesFromAttributeDir(path, pathPrefix, namePrefix, grammars)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml", ".xml":
		return discoverRoutesFromConfigPath(path, pathPrefix, namePrefix, visited, grammars)
	case ".php":
		if resourceType != "attribute" && resourceType != "" {
			return nil
		}
		return discoverRoutesFromAttributeFile(path, pathPrefix, namePrefix, grammars)
	default:
		return nil
	}
}

func discoverRoutesFromAttributeDir(root, pathPrefix, namePrefix string, grammars phpparser.GrammarRange) []Route {
	var routes []Route
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || filepath.Ext(path) != ".php" {
			return nil
		}
		routes = append(routes, discoverRoutesFromAttributeFile(path, pathPrefix, namePrefix, grammars)...)
		return nil
	})
	return routes
}

func discoverRoutesFromAttributeFile(path, pathPrefix, namePrefix string, grammars phpparser.GrammarRange) []Route {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	uri := path
	if abs, err := filepath.Abs(path); err == nil {
		uri = "file://" + filepath.ToSlash(abs)
	}

	routes := discoverRoutesInFile(uri, string(source), grammars)
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

func discoverRoutesInFile(uri, source string, grammars phpparser.GrammarRange) []Route {
	tree := parser.ParseTreeIn(source, grammars)
	reader := routeReader{src: source, file: parser.FileNodeOf(source, tree), lines: ast.NewLines(source)}
	var routes []Route
	ast.Walk(tree, func(n ast.Node) bool {
		class, ok := n.(*ast.ClassDecl)
		if !ok {
			return true
		}
		prefix := mergeRoutePrefix(reader.attributes(class.Attrs))
		for _, member := range class.Members {
			method, ok := member.(*ast.Method)
			if !ok {
				continue
			}
			for _, attr := range reader.attributes(method.Attrs) {
				if attr.Name == "" {
					continue
				}
				declRange := attr.AttrRange
				if attr.NameRange != (textpos.Range{}) {
					declRange = attr.NameRange
				}
				routes = append(routes, Route{
					Name:      prefix.Name + attr.Name,
					Path:      prefix.Path + attr.Path,
					URI:       uri,
					NameRange: attr.NameRange,
					DeclRange: declRange,
				})
			}
		}
		return true
	})
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

type routeReader struct {
	src   string
	file  *parser.FileNode
	lines ast.Lines
}

func (r routeReader) attributes(groups []*ast.AttributeGroup) []routeAttribute {
	var attrs []routeAttribute
	for _, group := range groups {
		for _, attr := range group.Attrs {
			if attr.Name == nil || !slices.ContainsFunc(routeAttributeFQNs, func(fqn string) bool {
				return strings.EqualFold(r.file.ResolveClassName(attr.Name.Bounds().Text(r.src)), fqn)
			}) {
				continue
			}
			route := routeAttribute{AttrRange: r.rangeOf(group.Span)}
			if attr.Args != nil {
				if name, ok := r.stringArgument(attr.Args, "name", 1); ok {
					route.Name, _ = parser.StringLiteral(name.Bounds().Text(r.src))
					route.NameRange = r.rangeOf(ast.Span{Start: name.Bounds().Start + 1, End: name.Bounds().End - 1})
				}
				if path, ok := r.stringArgument(attr.Args, "path", 0); ok {
					route.Path, _ = parser.StringLiteral(path.Bounds().Text(r.src))
				}
			}
			attrs = append(attrs, route)
		}
	}
	return attrs
}

func (r routeReader) stringArgument(args *ast.ArgList, name string, position int) (ast.Expr, bool) {
	var value ast.Expr
	if position < len(args.Args) && args.Args[position].Name == nil {
		value = args.Args[position].Value
	}
	for _, arg := range args.Args {
		if arg.Name != nil && arg.Name.Bounds().Text(r.src) == name {
			value = arg.Value
		}
	}
	lit, ok := value.(*ast.Literal)
	return lit, ok && lit.Kind == token.ConstantString
}

func (r routeReader) rangeOf(s ast.Span) textpos.Range {
	startLine, startCol := r.lines.Position(s.Start)
	endLine, endCol := r.lines.Position(s.End)
	return textpos.Range{
		Start: textpos.Position{Line: startLine, Character: startCol},
		End:   textpos.Position{Line: endLine, Character: endCol},
	}
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
			DeclRange: textpos.Range{Start: textpos.Position{Line: i, Character: 0}, End: textpos.Position{Line: i, Character: len(name)}},
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

func findXMLDeclRange(source, attr, value string) textpos.Range {
	needle := attr + `="` + value + `"`
	offset := strings.Index(source, needle)
	if offset < 0 {
		return textpos.Range{}
	}

	valueOffset := uint32(offset + len(attr) + 2)
	return routeReader{lines: ast.NewLines(source)}.rangeOf(ast.Span{Start: valueOffset, End: valueOffset + uint32(len(value))})
}
