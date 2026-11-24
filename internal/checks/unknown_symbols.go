package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// UnknownClassRule reports class names that resolve to no class-like: in
// `new`, `instanceof`, `::`, `catch`, `extends`, `implements` and attributes.
type UnknownClassRule struct {
	Shared *Shared
}

func (r *UnknownClassRule) Codes() []rule.Code { return []rule.Code{rule.UnknownClass} }

func (r *UnknownClassRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	syntax := r.Shared.tree(source, file.Grammar)
	var findings []Finding
	for _, ref := range syntax.references().classes {
		name := syntax.text(ref.name)
		line := syntax.line(ref.name)
		if classLikeKnown(name, line, file, index) || ref.attribute && isPHPBuiltinAttribute(name, line, file) {
			continue
		}
		kind := "class"
		if ref.attribute {
			kind = "attribute"
		}
		findings = append(findings, syntax.at(ref.name, Finding{
			Severity: rule.SeverityWarning,
			Code:     rule.UnknownClass,
			Message:  fmt.Sprintf("Unknown %s '%s'", kind, strings.TrimPrefix(name, "\\")),
		}))
	}
	return dropGuarded(findings, strings.Split(source, "\n"), syntax.extensionGuards())
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

// UnknownFunctionRule reports calls to functions that resolve to nothing.
type UnknownFunctionRule struct {
	Shared *Shared
}

func (r *UnknownFunctionRule) Codes() []rule.Code { return []rule.Code{rule.UnknownFunction} }

func (r *UnknownFunctionRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	syntax := r.Shared.tree(source, file.Grammar)
	var findings []Finding
	for _, call := range syntax.references().calls {
		name := syntax.text(call.name)
		if functionExists(name, file, index) {
			continue
		}
		// A PHP builtin the project's PHP version or extensions don't have is
		// reported once, by builtin-unavailable.
		if _, _, builtin := builtinFunctionAvailability(name, file); builtin {
			continue
		}
		findings = append(findings, syntax.at(call.name, Finding{
			Severity: rule.SeverityWarning,
			Code:     rule.UnknownFunction,
			Message:  fmt.Sprintf("Unknown function '%s'", strings.TrimPrefix(name, "\\")),
		}))
	}
	return dropGuarded(findings, strings.Split(source, "\n"), syntax.extensionGuards())
}

// UnknownMemberRule detects unresolved method/property/constant accesses when
// the receiver type can be resolved reliably.
type UnknownMemberRule struct {
	TypeResolver func(expr string, source string, line, col int, file *parser.FileNode) string
	// ConventionMethod, when set, reports whether a framework calls method on
	// instances of class (one class of the receiver's hierarchy) by convention,
	// without the class declaring it, as Laravel does with a notification's
	// toMail().
	ConventionMethod func(class, method string) bool
	// Shared, when set, shares the file's syntax tree with the other rules.
	Shared *Shared
}

func (r *UnknownMemberRule) Codes() []rule.Code { return []rule.Code{rule.UnknownMember} }

func (r *UnknownMemberRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || r.TypeResolver == nil {
		return nil
	}
	syntax := r.Shared.tree(source, file.Grammar)
	lines := strings.Split(source, "\n")
	var findings []Finding
	hierarchies := map[string]*memberHierarchy{}
	instanceofLines := linesWithInstanceof(lines)
	guards := memberGuards{file: file}
	guards.collect(syntax)

	for _, ref := range syntax.references().members {
		member := syntax.text(ref.name)
		// `Foo::class` and `$foo::class` are part of the language, not members.
		if ref.static && !ref.call() && strings.EqualFold(member, "class") {
			continue
		}
		ownerSpan := ref.owner.Bounds()
		ownerLine, ownerCol := syntax.lines.Position(ownerSpan.Start)
		lineNum, nameCol := syntax.lines.Position(ref.name.Start)
		if endLine, _ := syntax.lines.Position(ownerSpan.End); endLine != ownerLine || lineNum != ownerLine {
			continue
		}
		ownerExpr := syntax.text(ref.owner)
		// method_exists(), property_exists() and is_callable() are how code
		// asks whether an object has a member before using it.
		if guards.covers(ownerExpr, member, ref.call(), ref.static, lineNum, nameCol) {
			continue
		}
		owners := r.resolveOwners(ownerExpr, ownerLine, ownerCol, source, file, index)
		if len(owners) == 0 {
			continue
		}
		narrowed, ok := instanceofOwners(ownerExpr, lines, instanceofLines, lineNum, file, index)
		if !ok {
			continue
		}
		owners = appendNewOwners(owners, narrowed)
		access := memberAccess{name: member, isStatic: ref.static, isCall: ref.call()}
		// isset(), unset(), empty() and ?? are how code asks whether a property
		// is there; and a class with #[AllowDynamicProperties] (or stdClass)
		// has whatever properties code gives it.
		if !ref.call() && !ref.static && (ref.presence || anyOwnerAllowsDynamicProperties(owners, index)) {
			continue
		}
		if anyOwnerMayHaveMember(owners, access, file, index, hierarchies) || r.conventionMethod(owners, access, hierarchies) {
			continue
		}
		if anyImplementationHasMember(owners, isLateBoundOwner(ownerExpr), access, file, index, hierarchies) {
			continue
		}
		names := make([]string, len(owners))
		for i, owner := range owners {
			names[i] = unknownSymbolShortName(owner)
		}
		findings = append(findings, syntax.at(ref.name, Finding{
			Severity: rule.SeverityWarning,
			Code:     rule.UnknownMember,
			Message:  fmt.Sprintf("Unknown member '%s' on '%s'", member, strings.Join(names, "|")),
		}))
	}

	return findings
}

// resolveOwners returns every class-like a member may be accessed on. It returns
// nothing when any part of the type isn't known, since the member could be there.
func (r *UnknownMemberRule) resolveOwners(ownerExpr string, lineNum, col int, source string, file *parser.FileNode, index *symbols.Index) []string {
	if !strings.HasPrefix(ownerExpr, "$") && !strings.HasSuffix(ownerExpr, ")") && !strings.HasSuffix(ownerExpr, "]") {
		fqn, ok := resolveClassLikeName(ownerExpr, lineNum, file)
		if ok && fqn != "" {
			if name, found := canonicalClassLike(fqn, file, index); found {
				return []string{name}
			}
		}
	}

	return resolvedClassLikes(r.TypeResolver(ownerExpr, source, lineNum, col, file), file, index)
}

// instanceofOwners adds the classes an expression other than a plain
// variable ($this, a property, a call) is tested against before the access,
// since the code may be relying on that check: the resolver narrows plain
// variables only. The whole file so far is searched, which can only spare a
// finding. ok is false when a tested class is unknown, as the member could be
// on it.
func instanceofOwners(ownerExpr string, lines []string, instanceofLines []int, lineNum int, file *parser.FileNode, index *symbols.Index) (owners []string, ok bool) {
	if !strings.HasPrefix(ownerExpr, "$") || isPlainVariable(ownerExpr) {
		return nil, true
	}
	for _, i := range instanceofLines {
		if i > lineNum || i >= len(lines) {
			break
		}
		line := lines[i]
		if !strings.Contains(line, ownerExpr) {
			continue
		}
		for _, name := range instanceofTargets(line, ownerExpr) {
			fqn, resolved := resolveClassLikeName(name, lineNum, file)
			if !resolved || fqn == "" {
				return nil, false
			}
			canonical, found := canonicalClassLike(fqn, file, index)
			if !found {
				return nil, false
			}
			owners = append(owners, canonical)
		}
	}
	return owners, true
}

// appendNewOwners adds the owners not already listed, keeping first-seen order,
// so a class tested more than once is named once.
func appendNewOwners(owners, more []string) []string {
next:
	for _, owner := range more {
		for _, seen := range owners {
			if seen == owner {
				continue next
			}
		}
		owners = append(owners, owner)
	}
	return owners
}

// linesWithInstanceof lists the lines that test something with instanceof,
// which are few, so each member access only looks at those.
func linesWithInstanceof(lines []string) []int {
	var out []int
	for i, line := range lines {
		if strings.Contains(line, "instanceof") {
			out = append(out, i)
		}
	}
	return out
}

// instanceofTargets returns the class names `expr instanceof Name` tests
// expr against on a line, where expr is a whole variable, not the start of a
// longer name ($item, not $items).
func instanceofTargets(line, expr string) []string {
	var out []string
	for from := 0; ; {
		k := strings.Index(line[from:], expr)
		if k < 0 {
			return out
		}
		i := from + k + len(expr)
		from = i
		if i < len(line) && parser.IsIdentByte(line[i]) {
			continue
		}
		rest := strings.TrimLeft(line[i:], " \t")
		after, found := strings.CutPrefix(rest, "instanceof")
		if !found || after == "" || (after[0] != ' ' && after[0] != '\t') {
			continue
		}
		after = strings.TrimLeft(after, " \t")
		end := 0
		for end < len(after) && (parser.IsIdentByte(after[end]) || after[end] == '\\') {
			end++
		}
		if end == 0 || after[0] >= '0' && after[0] <= '9' {
			continue
		}
		out = append(out, after[:end])
	}
}

// isPlainVariable reports whether expr is a local variable, which the type
// resolver narrows by instanceof itself.
func isPlainVariable(expr string) bool {
	if len(expr) < 2 || expr[0] != '$' || expr == "$this" {
		return false
	}
	for i := 1; i < len(expr); i++ {
		if !parser.IsIdentByte(expr[i]) {
			return false
		}
	}
	return true
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
			(len(trimmed) == len(st.heredocLabel) || !parser.IsIdentByte(trimmed[len(st.heredocLabel)])) {
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
		if !parser.IsIdentByte(rest[k]) {
			return "", false
		}
	}
	return rest, true
}

// classLikeKnown is classLikeExists for unknown-class: a PHP builtin the
// project's PHP version or extensions don't have counts as known, since
// builtin-unavailable reports it, once.
func classLikeKnown(name string, line int, file *parser.FileNode, index *symbols.Index) bool {
	if classLikeExists(name, line, file, index) {
		return true
	}
	_, _, builtin := builtinClassAvailability(name, line, file)
	return builtin
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
	if _, ok = canonicalClassLike(fqn, file, index); ok {
		return true
	}
	// A class from an optional package the project suggests but doesn't
	// have installed is missing by design.
	return index != nil && index.IsOptionalClass(fqn)
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
	if sym := index.LookupFold(fqn, (*symbols.Symbol).IsClassLike); sym != nil {
		return sym.FQN, true
	}
	return "", false
}

func isFunctionSymbol(sym *symbols.Symbol) bool {
	return sym != nil && sym.Kind == symbols.KindFunction
}

func functionExists(name string, file *parser.FileNode, index *symbols.Index) bool {
	fqns, ok := resolve.FunctionNames(name, file)
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
	// subclass is set for a hierarchy looked at as a subclass of the receiver,
	// where a class declared inside a block still has its own members and its
	// parents': it doesn't stand for a class defined elsewhere.
	subclass string
}

// conventionMethod reports whether a framework calls the accessed method on
// one of owners, or a class it extends, by convention.
func (r *UnknownMemberRule) conventionMethod(owners []string, access memberAccess, hierarchies map[string]*memberHierarchy) bool {
	if r.ConventionMethod == nil || !access.isCall || access.isStatic {
		return false
	}
	for _, owner := range owners {
		if r.ConventionMethod(owner, access.name) {
			return true
		}
		if h := hierarchies[owner]; h != nil {
			for class := range h.visited {
				if r.ConventionMethod(class, access.name) {
					return true
				}
			}
		}
	}
	return false
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
	if !sym.IsClassLike() {
		h.open = true
		return
	}
	// A class declared inside a block is a placeholder or polyfill: the class
	// code runs with may be another one, with more members.
	if sym.IsConditional && fqn != h.subclass {
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
		if !index.Lookup(tr).IsClassLike() {
			h.open = true
		}
	}
	h.members = append(h.members, index.TraitMembers(fqn)...)
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
			if sym := index.Lookup(sub); sym != nil && sym.IsConditional {
				// As `if ($c) { class ftp extends ftp_sockets {} }`: the subclass
				// is one of the variants, whose members are its own and its parents'.
				key := "subclass|" + sub
				h, ok := hierarchies[key]
				if !ok {
					h = &memberHierarchy{visited: map[string]bool{}, subclass: sub}
					h.walk(sub, index)
					hierarchies[key] = h
				}
				if h.open || h.answers(access) {
					return true
				}
				continue
			}
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
			return resolve.ImportedName(cls.Extends, file.Namespace, file.Uses, ""), true
		}
		return "", false
	case "class", "readonly":
		// `new class ...` / `new readonly class ...` declare an anonymous class.
		return "", false
	}
	if symbols.IsPHPBuiltinType(lower) {
		return "", false
	}
	return resolve.ImportedName(name, file.Namespace, file.Uses, "class"), true
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
		// The `?` of a nullsafe `?->` is part of the chain, as in `$a?->b->c()`;
		// a ternary's isn't.
		if isExprChar(line[i]) || line[i] == '?' && strings.HasPrefix(line[i+1:], "->") {
			continue
		}
		return i + 1
	}
	return 0
}

func isExprChar(b byte) bool {
	return parser.IsIdentByte(b) || b == '$' || b == '\\' || b == ':' || b == '>' || b == '-' || b == ')' || b == '(' || b == ']' || b == '['
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
