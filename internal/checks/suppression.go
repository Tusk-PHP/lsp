package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

const (
	suppressionMarker = "@tusk-ignore"
	phpstanMarker     = "@phpstan-ignore"
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

type CommentSpan struct{ Start, End int }

func HasSuppressionMarker(source string) bool {
	return strings.Contains(source, suppressionMarker) || strings.Contains(source, phpstanMarker)
}

func PHPSuppressions(source string) Suppressions {
	if !HasSuppressionMarker(source) {
		return nil
	}
	var spans []CommentSpan
	for _, tok := range parser.Tokenize(source) {
		if tok.Kind == parser.TokenComment || tok.Kind == parser.TokenDocComment {
			spans = append(spans, CommentSpan{tok.Offset, tok.Offset + len(tok.Value)})
		}
	}
	return CommentSuppressions(source, spans)
}

func CommentSuppressions(source string, comments []CommentSpan) Suppressions {
	if !HasSuppressionMarker(source) {
		return nil
	}
	var out Suppressions
	line, counted := 0, 0
	for _, span := range comments {
		text := source[span.Start:span.End]
		if !HasSuppressionMarker(text) {
			continue
		}
		line += strings.Count(source[counted:span.Start], "\n")
		counted = span.Start
		nextLine := line + strings.Count(text, "\n") + 1
		own := ownLine(source, span)
		for _, m := range markers {
			at := markerIndex(text, m.text)
			if at < 0 {
				continue
			}
			list := codeList(text[at+len(m.text):])
			target, entry := line, suppressedCodes{}
			switch m.kind {
			case tuskIgnore:
				entry = suppressedCodes{all: len(list) == 0, targets: tuskTargets(list)}
				if own {
					target = nextLine
				}
			case phpstanIgnore:
				if len(list) == 0 {
					continue
				}
				entry.targets = phpstanTargets(list)
				if own {
					target = nextLine
				}
			case phpstanIgnoreLine:
				entry.all = true
			case phpstanIgnoreNextLine:
				entry.all, target = true, nextLine
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

func ownLine(source string, span CommentSpan) bool {
	lineStart := strings.LastIndexByte(source[:span.Start], '\n') + 1
	if strings.TrimSpace(source[lineStart:span.Start]) != "" {
		return false
	}
	rest := source[span.End:]
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
		rest = strings.TrimLeft(rest[n:], " \t")
		if !strings.HasPrefix(rest, ",") {
			return codes
		}
		rest = rest[1:]
	}
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
	return n
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
