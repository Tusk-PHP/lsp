package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/rule"
)

const (
	suppressionMarker = "@tusk-ignore"
	phpstanMarker     = "@phpstan-ignore"
	codeTrailers      = ".:-"
)

type markerKind int

const (
	tuskIgnore markerKind = iota
	phpstanIgnore
	phpstanIgnoreLine
	phpstanIgnoreNextLine
)

type marker struct {
	text    string
	kind    markerKind
	leading bool
}

var markers = []marker{
	{suppressionMarker, tuskIgnore, true},
	{phpstanMarker, phpstanIgnore, false},
	{phpstanMarker + "-line", phpstanIgnoreLine, false},
	{phpstanMarker + "-next-line", phpstanIgnoreNextLine, false},
}

var phpCommentOpeners = []string{"/**", "/*", "//", "#", "*"}

type Suppressions map[int]suppressedCodes

type suppressedCodes struct {
	all     bool
	targets []rule.Target
}

var phpstanLineTargets = []rule.Target{rule.ExternalTarget(rule.SourcePHPStan, ""), rule.AllNativeTarget()}

func HasSuppressionMarker(source string) bool {
	return strings.Contains(source, suppressionMarker) || strings.Contains(source, phpstanMarker)
}

func PHPSuppressions(source string, shared *Shared) Suppressions {
	if !HasSuppressionMarker(source) {
		return nil
	}
	return phpLayout(source, shared.lex(source)).Suppressions()
}

func (l Layout) Suppressions() Suppressions {
	var out Suppressions
	openers := l.commentOpeners()
	for _, span := range l.comments {
		text := l.source[span.Start:span.End]
		if !HasSuppressionMarker(text) {
			continue
		}
		for _, m := range markers {
			at := markerIndex(text, m, openers)
			if at < 0 {
				continue
			}
			list := codeList(text[at+len(m.text):])
			var entry suppressedCodes
			switch m.kind {
			case tuskIgnore:
				entry = suppressedCodes{all: len(list) == 0, targets: tuskTargets(list)}
			case phpstanIgnore:
				if len(list) == 0 {
					continue
				}
				entry.targets = phpstanTargets(list)
			default:
				entry.targets = phpstanLineTargets
			}
			for _, lines := range l.markerScope(span, at, m) {
				out = out.add(lines, entry)
			}
		}
	}
	return out
}

type lineRange struct{ first, last int }

func (l Layout) markerScope(span Span, at int, m marker) []lineRange {
	line := l.Line(span.Start)
	endLine := l.Line(span.End)
	text := l.source[span.Start:span.End]
	switch m.kind {
	case tuskIgnore:
		if !l.ownLine(span) {
			return []lineRange{{l.statementStart(line), l.statementEnd(line)}}
		}
		code, ok := l.reach(span.End)
		if !ok {
			return nil
		}
		return []lineRange{{endLine + 1, l.statementEnd(code)}}
	case phpstanIgnore:
		if l.codeBefore(span.Start) {
			return []lineRange{{line, line}}
		}
		if next := l.Line(l.nextNonComment(span.End)); next == endLine+1 {
			return []lineRange{{next, next}}
		}
		return []lineRange{{endLine, endLine}}
	case phpstanIgnoreLine:
		if isDocComment(text) {
			marker := l.Line(span.Start + at)
			return []lineRange{{marker, marker}}
		}
		return []lineRange{{endLine, endLine}}
	default:
		if !isDocComment(text) {
			return []lineRange{{endLine + 1, endLine + 1}}
		}
		next := l.Line(span.Start+at) + 1
		scope := []lineRange{{next, next}}
		if !hasTag(text[at+len(m.text):]) {
			scope = append(scope, lineRange{endLine + 1, endLine + 1})
		}
		return scope
	}
}

func (s Suppressions) add(lines lineRange, entry suppressedCodes) Suppressions {
	if s == nil {
		s = Suppressions{}
	}
	for line := lines.first; line <= lines.last; line++ {
		merged := s[line]
		merged.all = merged.all || entry.all
		merged.targets = append(merged.targets, entry.targets...)
		s[line] = merged
	}
	return s
}

func hasTag(text string) bool {
	for i := strings.IndexByte(text, '@'); i >= 0; i = strings.IndexByte(text, '@') {
		text = text[i+1:]
		if text != "" && isLetter(text[0]) {
			return true
		}
	}
	return false
}

func isDocComment(text string) bool {
	return len(text) > 3 && strings.HasPrefix(text, "/**") && strings.ContainsRune(" \t\r\n", rune(text[3]))
}

func (l Layout) ownLine(span Span) bool {
	if !l.startsLine(span.Start) {
		return false
	}
	rest := l.source[span.End:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	return strings.TrimSpace(rest) == ""
}

func codeList(rest string) []string {
	var codes []string
	for {
		rest = strings.TrimLeft(rest, " \t")
		n := codeLength(rest)
		if n == 0 {
			return codes
		}
		codes = append(codes, rest[:n])
		rest = skipReason(strings.TrimLeft(rest[n:], " \t"))
		if !strings.HasPrefix(rest, ",") {
			return codes
		}
		rest = rest[1:]
	}
}

func skipReason(rest string) string {
	if !strings.HasPrefix(rest, "(") {
		return rest
	}
	depth := 0
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.TrimLeft(rest[i+1:], " \t")
			}
		}
	}
	return ""
}

func markerIndex(comment string, m marker, openers []string) int {
	from := 0
	for {
		i := strings.Index(comment[from:], m.text)
		if i < 0 {
			return -1
		}
		i += from
		end := i + len(m.text)
		before := i == 0 || !isCodeByte(comment[i-1])
		after := end == len(comment) || comment[end] == ':' || !isCodeByte(comment[end])
		if before && after && (!m.leading || startsComment(comment, i, openers)) {
			return i
		}
		from = end
	}
}

func startsComment(comment string, at int, openers []string) bool {
	lead := strings.TrimLeft(comment[strings.LastIndexByte(comment[:at], '\n')+1:at], " \t")
	for _, opener := range openers {
		if strings.HasPrefix(lead, opener) {
			lead = lead[len(opener):]
			break
		}
	}
	return strings.TrimLeft(lead, " \t") == ""
}

func codeLength(s string) int {
	if s == "" || !isLetter(s[0]) {
		return 0
	}
	n := 1
	for n < len(s) && isCodeByte(s[n]) {
		n++
	}
	return len(strings.TrimRight(s[:n], codeTrailers))
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isCodeByte(c byte) bool {
	return isLetter(c) || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':'
}

func tuskTargets(names []string) []rule.Target {
	targets := make([]rule.Target, 0, len(names))
	for _, name := range names {
		if target, err := rule.ParseTarget(name); err == nil {
			targets = append(targets, target)
		}
	}
	return targets
}

func (s Suppressions) Suppresses(line int, source, code string) bool {
	entry, ok := s[line]
	if !ok {
		return false
	}
	if entry.all {
		return true
	}
	for _, target := range entry.targets {
		if target.Matches(source, code) {
			return true
		}
	}
	return false
}

func SuppressionComment(target rule.Target) string {
	return suppressionMarker + " " + target.String()
}

func PHPStanSuppressionComment(identifier string) string {
	return phpstanMarker + " " + identifier
}
