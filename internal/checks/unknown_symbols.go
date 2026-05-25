package checks

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
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
	inBlockComment := false

	for lineNum, raw := range lines {
		line := maskPHPLine(raw, &inBlockComment)
		findings = append(findings, checkUnknownClassMatches(line, lineNum, file, index, unknownClassNewRe)...)
		findings = append(findings, checkUnknownClassMatches(line, lineNum, file, index, unknownClassSingleRefRe)...)
		findings = append(findings, checkUnknownStaticClassMatches(line, lineNum, file, index)...)
		findings = append(findings, checkUnknownCatchClasses(line, lineNum, file, index)...)
		findings = append(findings, checkUnknownImplementsClasses(line, lineNum, file, index)...)
	}

	return findings
}

func checkUnknownClassMatches(line string, lineNum int, file *parser.FileNode, index *symbols.Index, re *regexp.Regexp) []Finding {
	var findings []Finding
	for _, match := range re.FindAllStringSubmatchIndex(line, -1) {
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

// UnknownFunctionRule detects unresolved plain function calls.
type UnknownFunctionRule struct{}

func (r *UnknownFunctionRule) Code() string { return "unknown-function" }

var unknownFunctionCallRe = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_\\]*)\s*\(`)

var skippedFunctionLikeKeywords = map[string]bool{
	"if": true, "elseif": true, "for": true, "foreach": true, "while": true,
	"switch": true, "catch": true, "echo": true, "empty": true, "isset": true,
	"unset": true, "include": true, "include_once": true, "require": true,
	"require_once": true, "array": true, "match": true, "clone": true,
	"eval": true, "print": true, "die": true, "exit": true,
}

func (r *UnknownFunctionRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding
	inBlockComment := false

	for lineNum, raw := range lines {
		line := maskPHPLine(raw, &inBlockComment)
		for _, match := range unknownFunctionCallRe.FindAllStringSubmatchIndex(line, -1) {
			start, end := match[2], match[3]
			name := line[start:end]
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

	return findings
}

func shouldSkipFunctionCall(line string, start int, name string) bool {
	lower := strings.ToLower(name)
	if skippedFunctionLikeKeywords[lower] {
		return true
	}
	if start >= 2 && line[start-2:start] == "->" {
		return true
	}
	if start >= 2 && line[start-2:start] == "::" {
		return true
	}
	prefix := strings.TrimSpace(line[:start])
	if prefix == "" {
		return false
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
	inBlockComment := false

	for lineNum, raw := range lines {
		line := maskPHPLine(raw, &inBlockComment)
		for _, match := range unknownMemberAccessRe.FindAllStringSubmatchIndex(line, -1) {
			opStart := match[2]
			nameStart, nameEnd := match[4], match[5]
			member := line[nameStart:nameEnd]
			ownerExpr := strings.TrimSpace(line[backwardExprStart(line, opStart):opStart])
			if ownerExpr == "" {
				continue
			}

			ownerFQN, ok := r.resolveOwnerFQN(ownerExpr, lineNum, source, file, index)
			if !ok || ownerFQN == "" {
				continue
			}
			if !memberExists(ownerFQN, member, file, index) {
				findings = append(findings, Finding{
					StartLine: lineNum,
					StartCol:  nameStart,
					EndLine:   lineNum,
					EndCol:    nameEnd,
					Severity:  SeverityWarning,
					Code:      "unknown-member",
					Message:   fmt.Sprintf("Unknown member '%s' on '%s'", member, unknownSymbolShortName(ownerFQN)),
				})
			}
		}
	}

	return findings
}

func (r *UnknownMemberRule) resolveOwnerFQN(ownerExpr string, lineNum int, source string, file *parser.FileNode, index *symbols.Index) (string, bool) {
	if !strings.HasPrefix(ownerExpr, "$") && !strings.HasSuffix(ownerExpr, ")") && !strings.HasSuffix(ownerExpr, "]") {
		fqn, ok := resolveClassLikeName(ownerExpr, lineNum, file)
		if ok && fqn != "" {
			if localClassLikeExists(fqn, file) {
				return fqn, true
			}
			if index != nil && isClassLikeSymbol(index.Lookup(fqn)) {
				return fqn, true
			}
		}
	}

	resolved := r.TypeResolver(ownerExpr, source, lineNum, file)
	fqn, ok := normalizeResolvedClassLike(resolved, lineNum, file)
	if !ok || fqn == "" {
		return "", false
	}
	if localClassLikeExists(fqn, file) {
		return fqn, true
	}
	if index == nil || !isClassLikeSymbol(index.Lookup(fqn)) {
		return "", false
	}
	return fqn, true
}

func maskPHPLine(line string, inBlockComment *bool) string {
	if line == "" {
		return line
	}
	buf := []byte(line)
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(buf); i++ {
		if *inBlockComment {
			buf[i] = ' '
			if i+1 < len(buf) && line[i] == '*' && line[i+1] == '/' {
				buf[i+1] = ' '
				*inBlockComment = false
				i++
			}
			continue
		}
		if inSingle {
			buf[i] = ' '
			if escaped {
				escaped = false
				continue
			}
			if line[i] == '\\' {
				escaped = true
				continue
			}
			if line[i] == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			buf[i] = ' '
			if escaped {
				escaped = false
				continue
			}
			if line[i] == '\\' {
				escaped = true
				continue
			}
			if line[i] == '"' {
				inDouble = false
			}
			continue
		}

		if i+1 < len(buf) && line[i] == '/' && line[i+1] == '*' {
			buf[i] = ' '
			buf[i+1] = ' '
			*inBlockComment = true
			i++
			continue
		}
		if i+1 < len(buf) && line[i] == '/' && line[i+1] == '/' {
			for j := i; j < len(buf); j++ {
				buf[j] = ' '
			}
			break
		}
		if line[i] == '#' {
			for j := i; j < len(buf); j++ {
				buf[j] = ' '
			}
			break
		}
		if line[i] == '\'' {
			buf[i] = ' '
			inSingle = true
			continue
		}
		if line[i] == '"' {
			buf[i] = ' '
			inDouble = true
			continue
		}
	}

	return string(buf)
}

func classLikeExists(name string, line int, file *parser.FileNode, index *symbols.Index) bool {
	fqn, ok := resolveClassLikeName(name, line, file)
	if !ok || fqn == "" {
		return true
	}
	if localClassLikeExists(fqn, file) {
		return true
	}
	if index == nil {
		return false
	}
	return isClassLikeSymbol(index.Lookup(fqn))
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
		if sym := index.Lookup(fqn); sym != nil && sym.Kind == symbols.KindFunction {
			return true
		}
	}
	return false
}

func memberExists(ownerFQN, member string, file *parser.FileNode, index *symbols.Index) bool {
	if localMemberExists(ownerFQN, member, file) {
		return true
	}
	if index == nil {
		return false
	}
	for _, sym := range index.GetClassMembers(ownerFQN) {
		if sym.Name == member || sym.Name == "$"+member {
			return true
		}
	}
	return false
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
	if file == nil {
		return false
	}
	for _, cls := range file.Classes {
		if classNodeFQN(&cls, file) == fqn {
			return true
		}
	}
	for _, iface := range file.Interfaces {
		if interfaceNodeFQN(&iface, file) == fqn {
			return true
		}
	}
	for _, tr := range file.Traits {
		if traitNodeFQN(&tr, file) == fqn {
			return true
		}
	}
	for _, en := range file.Enums {
		if enumNodeFQN(&en, file) == fqn {
			return true
		}
	}
	return false
}

func localFunctionExists(fqn string, file *parser.FileNode) bool {
	if file == nil {
		return false
	}
	for _, fn := range file.Functions {
		if functionNodeFQN(&fn, file) == fqn {
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
			if m.Name == member {
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
			if m.Name == member {
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

func normalizeResolvedClassLike(typ string, line int, file *parser.FileNode) (string, bool) {
	typ = strings.TrimSpace(typ)
	if typ == "" || typ == "mixed" || typ == "null" {
		return "", false
	}
	if strings.HasPrefix(typ, "?") {
		typ = typ[1:]
	}
	if idx := strings.Index(typ, "<"); idx >= 0 {
		typ = typ[:idx]
	}
	if strings.Contains(typ, "|") {
		for _, part := range strings.Split(typ, "|") {
			part = strings.TrimSpace(part)
			if part == "" || part == "null" || part == "mixed" {
				continue
			}
			if fqn, ok := resolveClassLikeName(part, line, file); ok {
				return fqn, true
			}
		}
		return "", false
	}
	return resolveClassLikeName(typ, line, file)
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
	if idx := strings.LastIndex(fqn, "\\"); idx >= 0 {
		return fqn[idx+1:]
	}
	return fqn
}
