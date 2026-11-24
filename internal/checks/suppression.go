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

var markers = []struct {
	text string
	kind markerKind
}{
	{suppressionMarker, tuskIgnore},
	{phpstanMarker, phpstanIgnore},
	{phpstanMarker + "-line", phpstanIgnoreLine},
	{phpstanMarker + "-next-line", phpstanIgnoreNextLine},
}

type Suppressions map[int]suppressedCodes

type suppressedCodes struct {
	all     bool
	targets []rule.Target
}

var phpstanLineTargets = []rule.Target{rule.ExternalTarget(rule.SourcePHPStan, ""), rule.AllNativeTarget()}

func HasSuppressionMarker(source string) bool {
	return strings.Contains(source, suppressionMarker) || strings.Contains(source, phpstanMarker)
}

func PHPSuppressions(source string) Suppressions {
	if !HasSuppressionMarker(source) {
		return nil
	}
	return PHPLayout(source).Suppressions()
}

func BladeSuppressions(source string) Suppressions {
	if !HasSuppressionMarker(source) {
		return nil
	}
	return BladeLayout(source).Suppressions()
}

func (l Layout) Suppressions() Suppressions {
	var out Suppressions
	for _, span := range l.comments {
		text := l.source[span.Start:span.End]
		if !HasSuppressionMarker(text) {
			continue
		}
		line := l.Line(span.Start)
		next := line
		if l.ownLine(span) {
			next = l.nextCodeLine(span.End)
		}
		for _, m := range markers {
			at := markerIndex(text, m.text)
			if at < 0 {
				continue
			}
			list := codeList(text[at+len(m.text):])
			target, entry := next, suppressedCodes{}
			switch m.kind {
			case tuskIgnore:
				entry = suppressedCodes{all: len(list) == 0, targets: tuskTargets(list)}
			case phpstanIgnore:
				if len(list) == 0 {
					continue
				}
				entry.targets = phpstanTargets(list)
			case phpstanIgnoreLine:
				target, entry.targets = line, phpstanLineTargets
			case phpstanIgnoreNextLine:
				target, entry.targets = l.nextCodeLine(span.End), phpstanLineTargets
			}
			if out == nil {
				out = Suppressions{}
			}
			merged := out[target]
			merged.all = merged.all || entry.all
			merged.targets = append(merged.targets, entry.targets...)
			out[target] = merged
		}
	}
	return out
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

func markerIndex(comment, marker string) int {
	from := 0
	for {
		i := strings.Index(comment[from:], marker)
		if i < 0 {
			return -1
		}
		i += from
		end := i + len(marker)
		before := i == 0 || !isCodeByte(comment[i-1])
		after := end == len(comment) || !isCodeByte(comment[end])
		if before && after {
			return i
		}
		from = end
	}
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
