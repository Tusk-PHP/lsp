package composer

import (
	"strconv"
	"strings"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type PHPVersion struct {
	Major, Minor int
}

func (v PHPVersion) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor)
}

func (v PHPVersion) less(o PHPVersion) bool {
	return v.Major < o.Major || v.Major == o.Major && v.Minor < o.Minor
}

type PHPRange struct {
	Oldest   PHPVersion
	Grammars phpparser.GrammarRange
}

type release [3]int

func (r release) less(o release) bool {
	for i := range r {
		if r[i] != o[i] {
			return r[i] < o[i]
		}
	}
	return false
}

type ceiling struct {
	at        release
	inclusive bool
	bounded   bool
}

func (c ceiling) lower(o ceiling) ceiling {
	switch {
	case !o.bounded:
		return c
	case !c.bounded, o.at.less(c.at), o.at == c.at && !o.inclusive:
		return o
	}
	return c
}

func (c ceiling) newest() (PHPVersion, bool) {
	if !c.bounded {
		return PHPVersion{Major: int(^uint(0) >> 1)}, true
	}
	v := PHPVersion{Major: c.at[0], Minor: c.at[1]}
	switch {
	case c.inclusive || c.at[2] > 0:
	case v.Minor > 0:
		v.Minor--
	default:
		v.Major--
		v.Minor = int(^uint(0) >> 1)
	}
	return v, v.Major >= 0
}

func grammarOf(v PHPVersion) token.Version {
	switch {
	case v.Major < 8:
		return token.PHP74
	case v.Major > 8 || v.Minor > int(token.Latest-token.PHP80):
		return token.Latest
	}
	return token.PHP80 + token.Version(v.Minor)
}

func ParsePHPConstraint(constraint string) (PHPRange, bool) {
	var result PHPRange
	var newest PHPVersion
	found := false
	for _, alternative := range strings.Split(strings.ReplaceAll(constraint, "||", "|"), "|") {
		floor, top, ok := parseAlternative(alternative)
		if !ok {
			continue
		}
		latest, ok := top.newest()
		oldest := PHPVersion{Major: floor[0], Minor: floor[1]}
		if !ok || latest.less(oldest) {
			continue
		}
		if !found || oldest.less(result.Oldest) {
			result.Oldest = oldest
		}
		if !found || newest.less(latest) {
			newest = latest
		}
		found = true
	}
	result.Grammars = phpparser.GrammarsBetween(grammarOf(result.Oldest), grammarOf(newest))
	return result, found
}

func parseAlternative(alternative string) (release, ceiling, bool) {
	fields := strings.Fields(strings.ReplaceAll(alternative, ",", " "))
	if len(fields) == 3 && fields[1] == "-" {
		from, _, okFrom := parseRelease(fields[0])
		to, given, okTo := parseRelease(fields[2])
		if !okFrom || !okTo {
			return release{}, ceiling{}, false
		}
		if given == len(to) {
			return from, ceiling{at: to, inclusive: true, bounded: true}, true
		}
		return from, ceiling{at: bump(to, given-1), bounded: true}, true
	}
	var floor release
	var top ceiling
	terms := 0
	for i := 0; i < len(fields); i++ {
		term := fields[i]
		if strings.Trim(term, "<>=!^~") == "" && i+1 < len(fields) {
			i++
			term += fields[i]
		}
		low, high, ok := parseTerm(term)
		if !ok {
			return release{}, ceiling{}, false
		}
		if floor.less(low) {
			floor = low
		}
		top = top.lower(high)
		terms++
	}
	return floor, top, terms > 0
}

func parseTerm(term string) (release, ceiling, bool) {
	if term == "*" || term == "x" {
		return release{}, ceiling{}, true
	}
	op := term[:len(term)-len(strings.TrimLeft(term, "<>=!^~"))]
	v, given, ok := parseRelease(term[len(op):])
	if !ok {
		return release{}, ceiling{}, false
	}
	wildcard := given < 0
	if wildcard {
		given = -given - 1
		if op != "" && op != "=" && op != "==" {
			return release{}, ceiling{}, false
		}
	}
	switch op {
	case ">=", ">":
		return v, ceiling{}, true
	case "<=":
		return release{}, ceiling{at: v, inclusive: true, bounded: true}, true
	case "<":
		return release{}, ceiling{at: v, bounded: true}, true
	case "!=":
		return release{}, ceiling{}, true
	case "^":
		major := 0
		for major < given-1 && v[major] == 0 {
			major++
		}
		return v, ceiling{at: bump(v, major), bounded: true}, true
	case "~":
		return v, ceiling{at: bump(v, max(given-2, 0)), bounded: true}, true
	case "", "=", "==":
		if wildcard {
			if given == 0 {
				return release{}, ceiling{}, true
			}
			return v, ceiling{at: bump(v, given-1), bounded: true}, true
		}
		return v, ceiling{at: v, inclusive: true, bounded: true}, true
	}
	return release{}, ceiling{}, false
}

func bump(v release, at int) release {
	var out release
	copy(out[:at], v[:at])
	out[at] = v[at] + 1
	return out
}

func parseRelease(s string) (release, int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-@+"); i >= 0 {
		s = s[:i]
	}
	var v release
	parts := strings.Split(s, ".")
	if len(parts) > len(v) {
		parts = parts[:len(v)]
	}
	for i, part := range parts {
		if part == "*" || part == "x" || part == "X" {
			return v, -i - 1, true
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return release{}, 0, false
		}
		v[i] = n
	}
	return v, len(parts), true
}
