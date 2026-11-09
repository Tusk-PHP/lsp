package checks

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// UnknownClassRule detects unresolved class-like references in a narrow set of
// stable runtime and declaration contexts.
type UnknownClassRule struct{}

func (r *UnknownClassRule) Code() string { return "unknown-class" }

var unknownClassNewRe = regexp.MustCompile(`\bnew\s+(\\?[A-Za-z_][A-Za-z0-9_\\]*)`)
var unknownClassSingleRefRe = regexp.MustCompile(`\b(?:extends|instanceof)\s+(\\?[A-Za-z_][A-Za-z0-9_\\]*)`)
var unknownClassStaticRe = regexp.MustCompile(`(^|[^$A-Za-z0-9_\\])(\\?[A-Za-z_][A-Za-z0-9_\\]*)::`)
var unknownClassCatchRe = regexp.MustCompile(`\bcatch\s*\(\s*([^)]+)\s+\$[A-Za-z_][A-Za-z0-9_]*`)
var unknownClassImplementsRe = regexp.MustCompile(`\bimplements\s+([^{]+)`)

func (r *UnknownClassRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding
	mask := newMaskState(source)

	for lineNum, raw := range lines {
		line := maskPHPLine(raw, &mask)
		findings = append(findings, checkUnknownClassMatches(line, lineNum, file, index, unknownClassNewRe)...)
		findings = append(findings, checkUnknownClassMatches(line, lineNum, file, index, unknownClassSingleRefRe)...)
		findings = append(findings, checkUnknownStaticClassMatches(line, lineNum, file, index)...)
		findings = append(findings, checkUnknownCatchClasses(line, lineNum, file, index)...)
		findings = append(findings, checkUnknownImplementsClasses(line, lineNum, file, index)...)
		findings = append(findings, checkUnknownAttributeClasses(line, lineNum, file, index)...)
	}

	return dropGuarded(findings, lines, findExtensionGuards(source))
}

func checkUnknownClassMatches(line string, lineNum int, file *parser.FileNode, index *symbols.Index, re *regexp.Regexp) []Finding {
	var findings []Finding
	for _, match := range re.FindAllStringSubmatchIndex(line, -1) {
		if !isKeywordAt(line, match[0]) {
			continue
		}
		start, end := match[2], match[3]
		name := line[start:end]
		if classLikeExists(name, lineNum, file, index) {
			continue
		}
		findings = append(findings, Finding{
			StartLine: lineNum,
			StartCol:  start,
			EndLine:   lineNum,
			EndCol:    end,
			Severity:  SeverityWarning,
			Code:      "unknown-class",
			Message:   fmt.Sprintf("Unknown class '%s'", strings.TrimPrefix(name, "\\")),
		})
	}
	return findings
}

// isKeywordAt reports whether the word starting at line[i] is a keyword rather
// than part of a variable or member name: `$new as`, `$this->extends`,
// `Foo::catch`. The regexes' \b treats `$` and `>` as word boundaries.
func isKeywordAt(line string, i int) bool {
	if i <= 0 {
		return true
	}
	switch line[i-1] {
	case '$':
		return false
	case '>':
		return i < 2 || line[i-2] != '-'
	case ':':
		return i < 2 || line[i-2] != ':'
	}
	return true
}

func checkUnknownStaticClassMatches(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, match := range unknownClassStaticRe.FindAllStringSubmatchIndex(line, -1) {
		start, end := match[4], match[5]
		name := line[start:end]
		if classLikeExists(name, lineNum, file, index) {
			continue
		}
		findings = append(findings, Finding{
			StartLine: lineNum,
			StartCol:  start,
			EndLine:   lineNum,
			EndCol:    end,
			Severity:  SeverityWarning,
			Code:      "unknown-class",
			Message:   fmt.Sprintf("Unknown class '%s'", strings.TrimPrefix(name, "\\")),
		})
	}
	return findings
}

func checkUnknownCatchClasses(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, match := range unknownClassCatchRe.FindAllStringSubmatchIndex(line, -1) {
		if !isKeywordAt(line, match[0]) {
			continue
		}
		listStart, listEnd := match[2], match[3]
		list := line[listStart:listEnd]
		offset := 0
		for _, part := range strings.Split(list, "|") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				offset += len(part) + 1
				continue
			}
			rel := strings.Index(part, trimmed)
			start := listStart + offset + rel
			end := start + len(trimmed)
			if classLikeExists(trimmed, lineNum, file, index) {
				offset += len(part) + 1
				continue
			}
			findings = append(findings, Finding{
				StartLine: lineNum,
				StartCol:  start,
				EndLine:   lineNum,
				EndCol:    end,
				Severity:  SeverityWarning,
				Code:      "unknown-class",
				Message:   fmt.Sprintf("Unknown class '%s'", strings.TrimPrefix(trimmed, "\\")),
			})
			offset += len(part) + 1
		}
	}
	return findings
}

func checkUnknownImplementsClasses(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, match := range unknownClassImplementsRe.FindAllStringSubmatchIndex(line, -1) {
		if !isKeywordAt(line, match[0]) {
			continue
		}
		listStart, listEnd := match[2], match[3]
		list := line[listStart:listEnd]
		offset := 0
		for _, part := range strings.Split(list, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				offset += len(part) + 1
				continue
			}
			rel := strings.Index(part, trimmed)
			start := listStart + offset + rel
			end := start + len(trimmed)
			if classLikeExists(trimmed, lineNum, file, index) {
				offset += len(part) + 1
				continue
			}
			findings = append(findings, Finding{
				StartLine: lineNum,
				StartCol:  start,
				EndLine:   lineNum,
				EndCol:    end,
				Severity:  SeverityWarning,
				Code:      "unknown-class",
				Message:   fmt.Sprintf("Unknown class '%s'", strings.TrimPrefix(trimmed, "\\")),
			})
			offset += len(part) + 1
		}
	}
	return findings
}

// checkUnknownAttributeClasses resolves each attribute name in `#[A, B(...), \\Ns\\C]`
// groups. Only the attribute class names are resolved — constructor arguments
// (including named arguments like `nullable: true` and array literals with
// their own commas) are skipped by the depth-aware scanner.
func checkUnknownAttributeClasses(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, g := range scanAttributeGroups(line) {
		for _, tok := range g.names {
			if classLikeExists(tok.name, lineNum, file, index) || isPHPBuiltinAttribute(tok.name, lineNum, file) {
				continue
			}
			findings = append(findings, Finding{
				StartLine: lineNum,
				StartCol:  tok.start,
				EndLine:   lineNum,
				EndCol:    tok.start + len(tok.name),
				Severity:  SeverityWarning,
				Code:      "unknown-class",
				Message:   fmt.Sprintf("Unknown attribute '%s'", strings.TrimPrefix(tok.name, "\\")),
			})
		}
	}
	return findings
}

// isPHPBuiltinAttribute reports whether an attribute resolves to one of PHP's
// own, such as #[AllowDynamicProperties] or #[\SensitiveParameter], from any
// PHP version. Those are forward-compatible: a PHP that doesn't know one
// ignores it, so it is never an unknown class. builtin-unavailable says when
// it has no effect yet. A name that resolves into a namespace is not PHP's.
func isPHPBuiltinAttribute(name string, line int, file *parser.FileNode) bool {
	fqn, ok := resolveClassLikeName(name, line, file)
	if !ok || fqn == "" || strings.Contains(fqn, "\\") {
		return false
	}
	_, known := symbols.LookupAvailability(fqn)
	return known
}

// attrToken is an attribute class name and the column where it begins.
type attrToken struct {
	name  string
	start int
}

// attrGroup is one PHP 8 attribute group `#[...]` on a line: its byte span
// (from `#` through the matching `]`) and the class names it declares.
type attrGroup struct {
	start int // index of `#`
	end   int // one past the closing `]` (or end of line if unterminated)
	names []attrToken
}

// scanAttributeGroups finds every `#[...]` group on a (string/comment-masked)
// line and extracts each attribute's class name. Commas, parentheses and
// brackets inside an attribute's argument list are tracked so that constructor
// arguments are never mistaken for additional attribute names — a single
// attribute like `Char("name", nullable: true)` yields just `Char`, and
// `Unique(["a", "b"])` yields just `Unique`.
func scanAttributeGroups(line string) []attrGroup {
	var groups []attrGroup
	for i := 0; i+1 < len(line); i++ {
		if line[i] != '#' || line[i+1] != '[' {
			continue
		}
		g := attrGroup{start: i}
		depth := 1 // inside the opening `[`
		paren := 0 // depth inside an argument list `(...)`
		segStart := i + 2
		flush := func(end int) {
			if end < segStart {
				return
			}
			seg := line[segStart:end]
			trimmed := strings.TrimSpace(seg)
			if trimmed == "" {
				return
			}
			name := trimmed
			if p := strings.IndexByte(name, '('); p >= 0 {
				name = strings.TrimSpace(name[:p])
			}
			if name == "" {
				return
			}
			rel := strings.Index(seg, name)
			g.names = append(g.names, attrToken{name: name, start: segStart + rel})
		}
		j := i + 2
		for ; j < len(line); j++ {
			c := line[j]
			if paren > 0 {
				switch c {
				case '(':
					paren++
				case ')':
					paren--
				}
				continue
			}
			switch c {
			case '(':
				paren++
			case '[':
				depth++
			case ']':
				depth--
			case ',':
				if depth == 1 {
					flush(j)
					segStart = j + 1
				}
			}
			if depth == 0 {
				flush(j)
				break
			}
		}
		if depth == 0 {
			g.end = j + 1
		} else {
			// Unterminated on this line (multi-line attribute): resolve the
			// trailing segment and treat the rest of the line as the group span.
			flush(j)
			g.end = len(line)
		}
		groups = append(groups, g)
		i = j
	}
	return groups
}

// posInAttributeGroup reports whether byte offset pos falls inside any
// attribute group span.
func posInAttributeGroup(pos int, groups []attrGroup) bool {
	for _, g := range groups {
		if pos >= g.start && pos < g.end {
			return true
		}
	}
	return false
}

// UnknownFunctionRule detects unresolved plain function calls.
type UnknownFunctionRule struct{}

func (r *UnknownFunctionRule) Code() string { return "unknown-function" }

var unknownFunctionCallRe = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_\\]*)\s*\(`)

var skippedFunctionLikeKeywords = map[string]bool{
	"if": true, "elseif": true, "for": true, "foreach": true, "while": true,
	"switch": true, "catch": true, "echo": true, "empty": true, "isset": true,
	"unset": true, "include": true, "include_once": true, "require": true,
	"require_once": true, "array": true, "match": true, "clone": true,
	"eval": true, "print": true, "die": true, "exit": true, "function": true,
	"fn": true, "use": true, "return": true, "list": true, "case": true,
	"and": true, "or": true, "xor": true, "instanceof": true, "throw": true,
	"yield": true, "static": true, "declare": true,
	// `new ($class)()`, `new\nclass($args)`, and the PHP 8.4 asymmetric
	// visibility and DNF types of `public private(set)` and
	// `private (A&B)|null $x` are not calls; none of these names can be a
	// function.
	"new": true, "class": true, "public": true, "protected": true, "private": true,
}

func (r *UnknownFunctionRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding
	mask := newMaskState(source)

	for lineNum, raw := range lines {
		line := maskPHPLine(raw, &mask)
		attrGroups := scanAttributeGroups(line)
		for _, match := range unknownFunctionCallRe.FindAllStringSubmatchIndex(line, -1) {
			start, end := match[2], match[3]
			name := line[start:end]
			// `#[Char(...)]` looks like a `Char(` call to the regex; the name is
			// an attribute class, not a function. Attribute argument lists are
			// constant expressions and cannot contain real function calls.
			if posInAttributeGroup(start, attrGroups) {
				continue
			}
			if shouldSkipFunctionCall(line, start, name) {
				continue
			}
			if functionExists(name, file, index) {
				continue
			}
			findings = append(findings, Finding{
				StartLine: lineNum,
				StartCol:  start,
				EndLine:   lineNum,
				EndCol:    end,
				Severity:  SeverityWarning,
				Code:      "unknown-function",
				Message:   fmt.Sprintf("Unknown function '%s'", strings.TrimPrefix(name, "\\")),
			})
		}
	}

	return dropGuarded(findings, lines, findExtensionGuards(source))
}

func shouldSkipFunctionCall(line string, start int, name string) bool {
	lower := strings.ToLower(name)
	if skippedFunctionLikeKeywords[lower] {
		return true
	}
	// `$callback()` calls a variable, not a function named callback.
	if start >= 1 && line[start-1] == '$' {
		return true
	}
	if start >= 2 && line[start-2:start] == "->" {
		return true
	}
	if start >= 2 && line[start-2:start] == "::" {
		return true
	}
	prefix := strings.TrimSpace(line[:start])
	// A property hook with a parameter, `set (array $value) { … }` or
	// `set (array $value) => …`, declares the hook; it doesn't call set().
	if (lower == "set" || lower == "get") && isHookDeclaration(line, start+len(name)) {
		return true
	}
	if prefix == "" {
		return false
	}
	// `yield from (expr)` delegates to a generator; it doesn't call from().
	if lower == "from" && hasKeywordSuffix(prefix, "yield") {
		return true
	}
	// `function &get()` declares a function that returns by reference.
	if trimmed := strings.TrimSuffix(prefix, "&"); trimmed != prefix {
		prefix = strings.TrimSpace(trimmed)
	}
	switch {
	case strings.HasSuffix(prefix, "function"),
		strings.HasSuffix(prefix, "fn"),
		strings.HasSuffix(prefix, "new"),
		strings.HasSuffix(prefix, "new \\"),
		strings.HasSuffix(prefix, "class"),
		strings.HasSuffix(prefix, "interface"),
		strings.HasSuffix(prefix, "trait"),
		strings.HasSuffix(prefix, "enum"):
		return true
	default:
		return false
	}
}

// isHookDeclaration reports whether the parenthesised list starting at or
// after pos (just past a `get`/`set` name) is followed on the same line by a
// hook body, `{` or `=>`, rather than ending a call.
func isHookDeclaration(line string, pos int) bool {
	i := pos
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || line[i] != '(' {
		return false
	}
	depth := 0
	for ; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				rest := strings.TrimSpace(line[i+1:])
				return strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "=>")
			}
		}
	}
	return false
}

// hasKeywordSuffix reports whether s ends with the word kw, not with a longer
// identifier ending in it.
func hasKeywordSuffix(s, kw string) bool {
	if !strings.HasSuffix(s, kw) {
		return false
	}
	if len(s) == len(kw) {
		return true
	}
	c := s[len(s)-len(kw)-1]
	return !(c == '_' || c == '$' || c == '\\' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))
}

// UnknownMemberRule detects unresolved method/property/constant accesses when
// the receiver type can be resolved reliably.
type UnknownMemberRule struct {
	TypeResolver func(expr string, source string, line int, file *parser.FileNode) string
}

func (r *UnknownMemberRule) Code() string { return "unknown-member" }

var unknownMemberAccessRe = regexp.MustCompile(`(\?\->|\->|::)\s*([A-Za-z_][A-Za-z0-9_]*)`)

func (r *UnknownMemberRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || r.TypeResolver == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding
	mask := newMaskState(source)
	hierarchies := map[string]*memberHierarchy{}

	for lineNum, raw := range lines {
		line := maskPHPLine(raw, &mask)
		for _, match := range unknownMemberAccessRe.FindAllStringSubmatchIndex(line, -1) {
			opStart, opEnd := match[2], match[3]
			nameStart, nameEnd := match[4], match[5]
			member := line[nameStart:nameEnd]
			isStatic := line[opStart:opEnd] == "::"
			// `Foo::class` and `$foo::class` are part of the language, not members.
			if isStatic && strings.EqualFold(member, "class") {
				continue
			}
			ownerExpr := strings.TrimSpace(line[backwardExprStart(line, opStart):opStart])
			// A line like `})->name('x')->middleware()` continues a chain begun on
			// an earlier line, so its owner can't be worked out from this line.
			if ownerExpr == "" || strings.Count(ownerExpr, ")") > strings.Count(ownerExpr, "(") {
				continue
			}

			owners := r.resolveOwners(ownerExpr, lineNum, source, file, index)
			if len(owners) == 0 {
				continue
			}
			narrowed, ok := instanceofOwners(ownerExpr, lines[:lineNum+1], lineNum, file, index)
			if !ok {
				continue
			}
			owners = append(owners, narrowed...)
			access := memberAccess{name: member, isStatic: isStatic, isCall: isCallAfter(line, nameEnd)}
			if !access.isCall && !isStatic {
				// isset(), unset(), empty() and ?? are how code asks whether a
				// property is there; and a class with #[AllowDynamicProperties]
				// (or stdClass) has whatever properties code gives it.
				if propertyCheckedForPresence(line, backwardExprStart(line, opStart), nameEnd) || anyOwnerAllowsDynamicProperties(owners, index) {
					continue
				}
			}
			if anyOwnerMayHaveMember(owners, access, file, index, hierarchies) {
				continue
			}
			if anyImplementationHasMember(owners, isLateBoundOwner(ownerExpr), access, file, index, hierarchies) {
				continue
			}
			names := make([]string, len(owners))
			for i, owner := range owners {
				names[i] = unknownSymbolShortName(owner)
			}
			findings = append(findings, Finding{
				StartLine: lineNum,
				StartCol:  nameStart,
				EndLine:   lineNum,
				EndCol:    nameEnd,
				Severity:  SeverityWarning,
				Code:      "unknown-member",
				Message:   fmt.Sprintf("Unknown member '%s' on '%s'", member, strings.Join(names, "|")),
			})
		}
	}

	return findings
}

// resolveOwners returns every class-like a member may be accessed on. It returns
// nothing when any part of the type isn't known, since the member could be there.
func (r *UnknownMemberRule) resolveOwners(ownerExpr string, lineNum int, source string, file *parser.FileNode, index *symbols.Index) []string {
	if !strings.HasPrefix(ownerExpr, "$") && !strings.HasSuffix(ownerExpr, ")") && !strings.HasSuffix(ownerExpr, "]") {
		fqn, ok := resolveClassLikeName(ownerExpr, lineNum, file)
		if ok && fqn != "" {
			if name, found := canonicalClassLike(fqn, file, index); found {
				return []string{name}
			}
		}
	}

	return resolvedClassLikes(r.TypeResolver(ownerExpr, source, lineNum, file), file, index)
}

// instanceofOwners adds the classes an expression is tested against before the
// access, since the code may be relying on that check. The whole file so far is
// searched, which can only spare a finding. ok is false when a tested class is
// unknown, as the member could be on it.
func instanceofOwners(ownerExpr string, lines []string, lineNum int, file *parser.FileNode, index *symbols.Index) (owners []string, ok bool) {
	if !strings.HasPrefix(ownerExpr, "$") {
		return nil, true
	}
	var re *regexp.Regexp
	for _, line := range lines {
		if !strings.Contains(line, ownerExpr) || !strings.Contains(line, "instanceof") {
			continue
		}
		if re == nil {
			re = regexp.MustCompile(regexp.QuoteMeta(ownerExpr) + `\s+instanceof\s+(\\?[A-Za-z_][A-Za-z0-9_\\]*)`)
		}
		for _, match := range re.FindAllStringSubmatch(line, -1) {
			fqn, resolved := resolveClassLikeName(match[1], lineNum, file)
			if !resolved || fqn == "" {
				return nil, false
			}
			name, found := canonicalClassLike(fqn, file, index)
			if !found {
				return nil, false
			}
			owners = append(owners, name)
		}
	}
	return owners, true
}

// resolvedClassLikes lists the class-likes in a type from the type resolver,
// which answers in fully qualified names, so they're looked up as they are.
// null and scalars are left out, as they have no members to find.
func resolvedClassLikes(typ string, file *parser.FileNode, index *symbols.Index) []string {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		return nil
	}
	var owners []string
	if !collectClassLikes(types.ParseTypeExpr(typ), file, index, &owners) {
		return nil
	}
	return owners
}

func collectClassLikes(expr types.TypeExpr, file *parser.FileNode, index *symbols.Index, owners *[]string) bool {
	switch expr.Kind {
	case types.TypeKindUnion, types.TypeKindIntersection:
		for _, part := range expr.Types {
			if !collectClassLikes(part, file, index, owners) {
				return false
			}
		}
		return true
	case types.TypeKindLiteral:
		return true
	case types.TypeKindNamed, types.TypeKindGeneric:
	default:
		return false
	}

	name := strings.TrimSpace(expr.Name)
	lower := strings.ToLower(name)
	switch lower {
	// `static` and friends are relative to the class that was called, which
	// isn't known here, and isn't necessarily the one being edited.
	case "mixed", "object", "callable", "iterable", "self", "static", "parent", "$this":
		return false
	}
	if symbols.IsPHPBuiltinType(lower) {
		return true
	}
	name, found := canonicalClassLike(strings.TrimPrefix(name, "\\"), file, index)
	if !found {
		return false
	}
	for _, owner := range *owners {
		if owner == name {
			return true
		}
	}
	*owners = append(*owners, name)
	return true
}

// maskState carries what maskPHPLine is inside of from one line to the next:
// inline HTML, a block comment, a quoted string or a heredoc/nowdoc body.
type maskState struct {
	inHTML         bool // outside <?php … ?>
	inBlockComment bool
	quote          byte   // '\'' or '"' while inside a string spanning lines
	heredocLabel   string // closing label while inside a heredoc/nowdoc body
}

// newMaskState returns the state for the first line of source. Text before
// the first open tag is inline HTML; a source with no open tag is all PHP.
func newMaskState(source string) maskState {
	return maskState{inHTML: parser.HasOpenTag(source)}
}

// maskPHPLine blanks comments and string contents on one line so text
// scanners only see code. Feed it every line of a file in order with the same
// state, so strings, heredocs and comments spanning lines stay masked.
func maskPHPLine(line string, st *maskState) string {
	if line == "" {
		return line
	}
	buf := []byte(line)
	i := 0

	if st.heredocLabel != "" {
		// Heredoc/nowdoc body: masked until a line starting (after
		// indentation) with the closing label.
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		if strings.HasPrefix(trimmed, st.heredocLabel) &&
			(len(trimmed) == len(st.heredocLabel) || !isIdentChar(trimmed[len(st.heredocLabel)])) {
			for j := 0; j < indent+len(st.heredocLabel); j++ {
				buf[j] = ' '
			}
			i = indent + len(st.heredocLabel)
			st.heredocLabel = ""
		} else {
			for j := range buf {
				buf[j] = ' '
			}
			return string(buf)
		}
	}

	escaped := false
	for ; i < len(buf); i++ {
		if st.inHTML {
			// Inline HTML (and any <script> in it) is not PHP: blank it up to
			// and including the next open tag.
			if n := parser.OpenTagLen(line, i); n > 0 {
				for j := i; j < i+n && j < len(buf); j++ {
					buf[j] = ' '
				}
				i += n - 1
				st.inHTML = false
				continue
			}
			buf[i] = ' '
			continue
		}
		if st.inBlockComment {
			buf[i] = ' '
			if i+1 < len(buf) && line[i] == '*' && line[i+1] == '/' {
				buf[i+1] = ' '
				st.inBlockComment = false
				i++
			}
			continue
		}
		if st.quote != 0 {
			buf[i] = ' '
			if escaped {
				escaped = false
				continue
			}
			if line[i] == '\\' {
				escaped = true
				continue
			}
			if line[i] == st.quote {
				st.quote = 0
			}
			continue
		}

		if i+1 < len(buf) && line[i] == '/' && line[i+1] == '*' {
			buf[i] = ' '
			buf[i+1] = ' '
			st.inBlockComment = true
			i++
			continue
		}
		if line[i] == '?' && i+1 < len(buf) && line[i+1] == '>' {
			// Close tag: back to inline HTML.
			buf[i] = ' '
			buf[i+1] = ' '
			i++
			st.inHTML = true
			continue
		}
		if (i+1 < len(buf) && line[i] == '/' && line[i+1] == '/') || (line[i] == '#' && (i+1 >= len(buf) || line[i+1] != '[')) {
			// `//` and `#` comments run to the end of the line or to a close
			// tag, whichever comes first. PHP 8 attribute syntax `#[Name]`
			// stays visible.
			j := i
			for ; j < len(buf) && !(line[j] == '?' && j+1 < len(buf) && line[j+1] == '>'); j++ {
				buf[j] = ' '
			}
			i = j - 1
			continue
		}
		if strings.HasPrefix(line[i:], "<<<") {
			if label, ok := heredocOpenLabel(line[i+3:]); ok {
				for j := i; j < len(buf); j++ {
					buf[j] = ' '
				}
				st.heredocLabel = label
				break
			}
		}
		if line[i] == '\'' || line[i] == '"' {
			buf[i] = ' '
			st.quote = line[i]
			continue
		}
	}

	return string(buf)
}

// heredocOpenLabel reads the label after `<<<` (`SQL`, `'SQL'` or `"SQL"`)
// when it is the last thing on the line.
func heredocOpenLabel(rest string) (string, bool) {
	rest = strings.TrimSpace(rest)
	if len(rest) >= 2 && (rest[0] == '\'' || rest[0] == '"') && rest[len(rest)-1] == rest[0] {
		rest = rest[1 : len(rest)-1]
	}
	if rest == "" || (rest[0] >= '0' && rest[0] <= '9') {
		return "", false
	}
	for k := 0; k < len(rest); k++ {
		if !isIdentChar(rest[k]) {
			return "", false
		}
	}
	return rest, true
}

func classLikeExists(name string, line int, file *parser.FileNode, index *symbols.Index) bool {
	// self, static and parent name the class being edited or its parent. If
	// that parent is missing, its `extends` is what's reported, not every
	// parent:: in the class.
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "self", "static", "parent":
		return true
	}
	fqn, ok := resolveClassLikeName(name, line, file)
	if !ok || fqn == "" {
		return true
	}
	_, ok = canonicalClassLike(fqn, file, index)
	return ok
}

// canonicalClassLike finds a class-like the way PHP does, ignoring case, and
// returns its name as declared, so later member lookups use the right key.
func canonicalClassLike(fqn string, file *parser.FileNode, index *symbols.Index) (string, bool) {
	if name, ok := localClassLikeFQN(fqn, file); ok {
		return name, true
	}
	if index == nil {
		return "", false
	}
	if sym := index.LookupFold(fqn, isClassLikeSymbol); sym != nil {
		return sym.FQN, true
	}
	return "", false
}

func isFunctionSymbol(sym *symbols.Symbol) bool {
	return sym != nil && sym.Kind == symbols.KindFunction
}

func functionExists(name string, file *parser.FileNode, index *symbols.Index) bool {
	fqns, ok := resolveFunctionNames(name, file)
	if !ok || len(fqns) == 0 {
		return true
	}
	for _, fqn := range fqns {
		if localFunctionExists(fqn, file) {
			return true
		}
		if index == nil {
			continue
		}
		if index.LookupFold(fqn, isFunctionSymbol) != nil {
			return true
		}
	}
	return false
}

// memberAccess describes how a member is used, which decides the magic methods
// that could answer it.
type memberAccess struct {
	name     string
	isStatic bool
	isCall   bool
}

// memberHierarchy is everything a class-like gets from its parents, traits and
// interfaces. It's open when something in it can answer for members we can't
// see, such as an unknown parent or an `@mixin`.
type memberHierarchy struct {
	members []*symbols.Symbol
	open    bool
	visited map[string]bool
}

func anyOwnerMayHaveMember(owners []string, access memberAccess, file *parser.FileNode, index *symbols.Index, hierarchies map[string]*memberHierarchy) bool {
	for _, owner := range owners {
		if ownerMayHaveMember(owner, access, file, index, hierarchies) {
			return true
		}
	}
	return false
}

func ownerMayHaveMember(owner string, access memberAccess, file *parser.FileNode, index *symbols.Index, hierarchies map[string]*memberHierarchy) bool {
	if localMemberExists(owner, access.name, file) {
		return true
	}
	h, ok := hierarchies[owner]
	if !ok {
		h = buildMemberHierarchy(owner, file, index)
		hierarchies[owner] = h
	}
	if h.open {
		return true
	}
	return h.answers(access)
}

func buildMemberHierarchy(owner string, file *parser.FileNode, index *symbols.Index) *memberHierarchy {
	h := &memberHierarchy{visited: map[string]bool{}}
	var sym *symbols.Symbol
	if index != nil {
		sym = index.Lookup(owner)
	}
	if sym == nil {
		// Declared in this file but not indexed yet: all we can vouch for is a
		// class standing on its own.
		h.open = !localClassStandsAlone(owner, file)
		return h
	}
	// An object typed by a vendor's interface usually does more than the interface
	// promises, as with `Storage::disk()` returning a Filesystem contract.
	if sym.Kind == symbols.KindInterface && sym.Source != symbols.SourceProject {
		h.open = true
		return h
	}
	h.walk(owner, index)
	return h
}

func (h *memberHierarchy) walk(fqn string, index *symbols.Index) {
	if h.visited[fqn] {
		return
	}
	h.visited[fqn] = true
	sym := index.Lookup(fqn)
	if !isClassLikeSymbol(sym) {
		h.open = true
		return
	}
	// A trait's `$this` is whichever class uses it, and builtins registered by
	// name alone have no members listed. Neither can show a member is missing.
	if sym.Kind == symbols.KindTrait || (symbols.IsBuiltin(sym) && len(sym.Children) == 0 && sym.Extends == "" && len(sym.Implements) == 0) {
		h.open = true
		return
	}
	if strings.Contains(sym.DocComment, "@mixin") {
		h.open = true
		return
	}
	// A generated interface gets members from configuration we don't read,
	// as Magento's extension attributes come from extension_attributes.xml.
	if sym.Kind == symbols.KindInterface && sym.Source == symbols.SourceGenerated {
		h.open = true
		return
	}
	h.members = append(h.members, sym.Children...)
	if sym.Kind == symbols.KindEnum {
		h.members = append(h.members, enumBuiltinMembers(sym)...)
	}
	if sym.Extends != "" {
		h.walk(sym.Extends, index)
	}
	for _, tr := range index.TraitsOf(fqn) {
		if isClassLikeSymbol(index.Lookup(tr)) {
			h.members = append(h.members, index.GetClassMembers(tr)...)
		} else {
			h.open = true
		}
	}
	for _, iface := range sym.Implements {
		h.walk(iface, index)
	}
}

func enumBuiltinMembers(sym *symbols.Symbol) []*symbols.Symbol {
	members := []*symbols.Symbol{
		{Name: "name", Kind: symbols.KindProperty},
		{Name: "cases", Kind: symbols.KindMethod},
	}
	if sym.BackedType != "" {
		members = append(members,
			&symbols.Symbol{Name: "value", Kind: symbols.KindProperty},
			&symbols.Symbol{Name: "from", Kind: symbols.KindMethod},
			&symbols.Symbol{Name: "tryFrom", Kind: symbols.KindMethod},
		)
	}
	return members
}

func (h *memberHierarchy) answers(access memberAccess) bool {
	for _, sym := range h.members {
		switch {
		case sym.Name == access.name || sym.Name == "$"+access.name:
			return true
		case sym.Kind == symbols.KindMethod && strings.EqualFold(sym.Name, access.name):
			// Method names are case-insensitive in PHP.
			return true
		case sym.Kind != symbols.KindMethod:
			continue
		}
		switch strings.ToLower(sym.Name) {
		case "__call", "__callstatic":
			if access.isCall {
				return true
			}
		case "__get", "__set":
			if !access.isCall && !access.isStatic {
				return true
			}
		}
	}
	return false
}

// isLateBoundOwner reports whether an access is resolved against the object's
// actual class at runtime, which can be any subclass: `$this->x` and
// `static::x`. `self::x` and `parent::x` are bound to the class written.
func isLateBoundOwner(ownerExpr string) bool {
	return ownerExpr == "$this" || strings.EqualFold(ownerExpr, "static")
}

// anyImplementationHasMember reports whether a class-like below one of the
// owners declares the member, when the object may be one of them.
//
// Through `$this` and `static::` (lateBound), the object is whichever subclass
// is in use: WordPress's ftp_base calls _exec(), which only its subclasses
// ftp_sockets and ftp_pure declare. An interface or an abstract class can't be
// instantiated either, so an object typed as one is always an implementation
// or a subclass, and code often calls what those add: Magento's
// `AbstractModel::getResource()` returns `AbstractDb`, whose EAV subclasses
// declare `getAttribute()`.
func anyImplementationHasMember(owners []string, lateBound bool, access memberAccess, file *parser.FileNode, index *symbols.Index, hierarchies map[string]*memberHierarchy) bool {
	if index == nil {
		return false
	}
	for _, owner := range owners {
		if !lateBound && !isAbstractClassLike(index.Lookup(owner)) {
			continue
		}
		for _, sub := range index.Implementations(owner) {
			if ownerMayHaveMember(sub, access, file, index, hierarchies) {
				return true
			}
		}
	}
	return false
}

// isAbstractClassLike reports whether sym is an interface or an abstract class,
// which no object is an instance of without being something below it.
func isAbstractClassLike(sym *symbols.Symbol) bool {
	if sym == nil {
		return false
	}
	return sym.Kind == symbols.KindInterface || (sym.Kind == symbols.KindClass && sym.IsAbstract)
}

// propertyCheckedForPresence reports whether the property access running
// from exprStart to nameEnd is the argument of isset(), unset() or empty(), or
// the left side of ??, where PHP doesn't require the property to exist.
func propertyCheckedForPresence(line string, exprStart, nameEnd int) bool {
	if rest := strings.TrimLeft(line[nameEnd:], " \t"); strings.HasPrefix(rest, "??") {
		return true
	}
	depth := 0
	for i := exprStart - 1; i >= 0; i-- {
		switch line[i] {
		case ')', ']':
			depth++
		case '(', '[':
			if depth > 0 {
				depth--
				continue
			}
			if line[i] == '[' {
				return false
			}
			j := i
			for j > 0 && (line[j-1] == ' ' || line[j-1] == '\t') {
				j--
			}
			k := j
			for k > 0 && isIdentChar(line[k-1]) {
				k--
			}
			switch strings.ToLower(line[k:j]) {
			case "isset", "unset", "empty":
				return true
			}
			return false
		case ';', '{', '}':
			return false
		}
	}
	return false
}

// anyOwnerAllowsDynamicProperties reports whether one of the owners, or a
// class it extends, is stdClass or has #[AllowDynamicProperties].
func anyOwnerAllowsDynamicProperties(owners []string, index *symbols.Index) bool {
	if index == nil {
		return false
	}
	for _, owner := range owners {
		classes := append([]string{owner}, index.GetInheritanceChain(owner)...)
		for _, cls := range classes {
			if strings.EqualFold(cls, "stdClass") {
				return true
			}
			sym := index.Lookup(cls)
			if sym == nil {
				continue
			}
			for _, attr := range sym.Attributes {
				if strings.EqualFold(strings.TrimPrefix(attr.FQN, "\\"), "AllowDynamicProperties") {
					return true
				}
			}
		}
	}
	return false
}

func isCallAfter(line string, pos int) bool {
	rest := strings.TrimLeft(line[pos:], " \t")
	return strings.HasPrefix(rest, "(")
}

func resolveClassLikeName(name string, line int, file *parser.FileNode) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	if strings.HasPrefix(name, "\\") {
		return strings.TrimPrefix(name, "\\"), true
	}
	lower := strings.ToLower(name)
	switch lower {
	case "self", "static":
		if cls := enclosingClassAtLine(file, line); cls != nil {
			return classNodeFQN(cls, file), true
		}
		return "", false
	case "parent":
		if cls := enclosingClassAtLine(file, line); cls != nil && cls.Extends != "" {
			return resolveImportedName(cls.Extends, file.Namespace, file.Uses, ""), true
		}
		return "", false
	case "class", "readonly":
		// `new class ...` / `new readonly class ...` declare an anonymous class.
		return "", false
	}
	if symbols.IsPHPBuiltinType(lower) {
		return "", false
	}
	return resolveImportedName(name, file.Namespace, file.Uses, "class"), true
}

func resolveFunctionNames(name string, file *parser.FileNode) ([]string, bool) {
	if file == nil {
		return nil, false
	}
	name = strings.TrimSpace(strings.TrimPrefix(name, "\\"))
	if name == "" {
		return nil, false
	}
	if strings.Contains(name, "\\") {
		return []string{resolveImportedName(name, file.Namespace, file.Uses, "function")}, true
	}
	for _, u := range file.Uses {
		if u.Kind == "function" && u.Alias == name {
			return []string{u.FullName}, true
		}
	}
	var fqns []string
	if file.Namespace != "" {
		fqns = append(fqns, file.Namespace+"\\"+name)
	}
	fqns = append(fqns, name)
	return fqns, true
}

func resolveImportedName(name, currentNs string, uses []parser.UseNode, useKind string) string {
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "\\") {
		return strings.TrimPrefix(name, "\\")
	}
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range uses {
		if useKind != "" && u.Kind != useKind {
			continue
		}
		if u.Alias != parts[0] {
			continue
		}
		if len(parts) > 1 {
			return u.FullName + "\\" + parts[1]
		}
		return u.FullName
	}
	if currentNs != "" {
		return currentNs + "\\" + name
	}
	return name
}

func localClassLikeExists(fqn string, file *parser.FileNode) bool {
	_, ok := localClassLikeFQN(fqn, file)
	return ok
}

// localClassLikeFQN finds a class-like declared in this file, ignoring case as
// PHP does, and returns its name as declared.
func localClassLikeFQN(fqn string, file *parser.FileNode) (string, bool) {
	if file == nil {
		return "", false
	}
	for _, cls := range file.Classes {
		if name := classNodeFQN(&cls, file); strings.EqualFold(name, fqn) {
			return name, true
		}
	}
	for _, iface := range file.Interfaces {
		if name := interfaceNodeFQN(&iface, file); strings.EqualFold(name, fqn) {
			return name, true
		}
	}
	for _, tr := range file.Traits {
		if name := traitNodeFQN(&tr, file); strings.EqualFold(name, fqn) {
			return name, true
		}
	}
	for _, en := range file.Enums {
		if name := enumNodeFQN(&en, file); strings.EqualFold(name, fqn) {
			return name, true
		}
	}
	return "", false
}

func localClassStandsAlone(fqn string, file *parser.FileNode) bool {
	if file == nil {
		return false
	}
	for i := range file.Classes {
		cls := &file.Classes[i]
		if classNodeFQN(cls, file) != fqn {
			continue
		}
		if cls.Extends != "" || len(cls.Implements) > 0 || len(cls.Traits) > 0 || strings.Contains(cls.DocComment, "@mixin") {
			return false
		}
		for _, m := range cls.Methods {
			if strings.HasPrefix(m.Name, "__") {
				switch strings.ToLower(m.Name) {
				case "__call", "__callstatic", "__get", "__set":
					return false
				}
			}
		}
		return true
	}
	for i := range file.Enums {
		if enumNodeFQN(&file.Enums[i], file) == fqn {
			return len(file.Enums[i].Implements) == 0
		}
	}
	return false
}

func localFunctionExists(fqn string, file *parser.FileNode) bool {
	if file == nil {
		return false
	}
	for _, fn := range file.Functions {
		if strings.EqualFold(functionNodeFQN(&fn, file), fqn) {
			return true
		}
	}
	return false
}

func localMemberExists(ownerFQN, member string, file *parser.FileNode) bool {
	if file == nil {
		return false
	}
	for _, cls := range file.Classes {
		if classNodeFQN(&cls, file) != ownerFQN {
			continue
		}
		for _, m := range cls.Methods {
			if strings.EqualFold(m.Name, member) {
				return true
			}
		}
		for _, p := range cls.Properties {
			if strings.TrimPrefix(p.Name, "$") == member {
				return true
			}
		}
		for _, c := range cls.Constants {
			if c.Name == member {
				return true
			}
		}
	}
	for _, en := range file.Enums {
		if enumNodeFQN(&en, file) != ownerFQN {
			continue
		}
		for _, m := range en.Methods {
			if strings.EqualFold(m.Name, member) {
				return true
			}
		}
		for _, c := range en.Cases {
			if c.Name == member {
				return true
			}
		}
	}
	return false
}

func enclosingClassAtLine(file *parser.FileNode, line int) *parser.ClassNode {
	if file == nil {
		return nil
	}
	// Only the line is known here: the line with the `{` counts as outside.
	if anon := file.AnonymousClassAt(line, 0); anon != nil {
		return anon
	}
	var best *parser.ClassNode
	for i := range file.Classes {
		cls := &file.Classes[i]
		if cls.StartLine <= line && (best == nil || cls.StartLine > best.StartLine) {
			best = cls
		}
	}
	return best
}

func classNodeFQN(cls *parser.ClassNode, file *parser.FileNode) string {
	if cls == nil {
		return ""
	}
	if cls.FullName != "" {
		return cls.FullName
	}
	if file != nil && file.Namespace != "" {
		return file.Namespace + "\\" + cls.Name
	}
	return cls.Name
}

func interfaceNodeFQN(iface *parser.InterfaceNode, file *parser.FileNode) string {
	if iface == nil {
		return ""
	}
	if iface.FullName != "" {
		return iface.FullName
	}
	if file != nil && file.Namespace != "" {
		return file.Namespace + "\\" + iface.Name
	}
	return iface.Name
}

func traitNodeFQN(tr *parser.TraitNode, file *parser.FileNode) string {
	if tr == nil {
		return ""
	}
	if tr.FullName != "" {
		return tr.FullName
	}
	if file != nil && file.Namespace != "" {
		return file.Namespace + "\\" + tr.Name
	}
	return tr.Name
}

func enumNodeFQN(en *parser.EnumNode, file *parser.FileNode) string {
	if en == nil {
		return ""
	}
	if en.FullName != "" {
		return en.FullName
	}
	if file != nil && file.Namespace != "" {
		return file.Namespace + "\\" + en.Name
	}
	return en.Name
}

func functionNodeFQN(fn *parser.FunctionNode, file *parser.FileNode) string {
	if fn == nil {
		return ""
	}
	if fn.FullName != "" {
		return fn.FullName
	}
	if file != nil && file.Namespace != "" {
		return file.Namespace + "\\" + fn.Name
	}
	return fn.Name
}

func isClassLikeSymbol(sym *symbols.Symbol) bool {
	if sym == nil {
		return false
	}
	switch sym.Kind {
	case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
		return true
	default:
		return false
	}
}

func backwardExprStart(line string, end int) int {
	depthParen := 0
	depthBracket := 0
	for i := end - 1; i >= 0; i-- {
		switch line[i] {
		case ')':
			depthParen++
		case '(':
			if depthParen == 0 {
				return i + 1
			}
			depthParen--
		case ']':
			depthBracket++
		case '[':
			if depthBracket == 0 {
				return i + 1
			}
			depthBracket--
		}
		if depthParen > 0 || depthBracket > 0 {
			continue
		}
		if isExprChar(line[i]) {
			continue
		}
		return i + 1
	}
	return 0
}

func isExprChar(b byte) bool {
	return isIdentChar(b) || b == '$' || b == '\\' || b == ':' || b == '>' || b == '-' || b == ')' || b == '(' || b == ']' || b == '['
}

func unknownSymbolShortName(fqn string) string {
	if parser.IsAnonymousClass(fqn) {
		return parser.AnonymousClassName
	}
	if idx := strings.LastIndex(fqn, "\\"); idx >= 0 {
		return fqn[idx+1:]
	}
	return fqn
}
