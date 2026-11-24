package rule

import (
	"fmt"
	"slices"
	"strings"
)

type Code string

const (
	SyntaxError           Code = "syntax-error"
	AbstractInConcrete    Code = "abstract-in-concrete"
	UnknownClass          Code = "unknown-class"
	UnknownFunction       Code = "unknown-function"
	UnknownMember         Code = "unknown-member"
	BuiltinUnavailable    Code = "builtin-unavailable"
	UnusedImport          Code = "unused-import"
	UnusedPrivateMethod   Code = "unused-private-method"
	UnusedPrivateProperty Code = "unused-private-property"
	UnusedPrivateConstant Code = "unused-private-constant"
	UnreachableCode       Code = "unreachable-code"
	RedundantUnionMember  Code = "redundant-union-member"
	RedundantNullsafe     Code = "redundant-nullsafe"
	UndefinedVariable     Code = "undefined-variable"
	UnusedVariable        Code = "unused-variable"
	Deprecated            Code = "deprecated"
	UnusedResult          Code = "unused-result"
	InvalidOverride       Code = "invalid-override"
	ArgumentCountMismatch Code = "argument-count-mismatch"
	UnknownNamedArgument  Code = "unknown-named-argument"
	UnknownColumn         Code = "unknown-column"
	UnknownRelation       Code = "unknown-relation"
	UnknownView           Code = "unknown-view"
	UnknownComponent      Code = "unknown-component"
	UnclosedDirective     Code = "unclosed-directive"
)

var All = []Code{
	SyntaxError,
	AbstractInConcrete,
	UnknownClass,
	UnknownFunction,
	UnknownMember,
	BuiltinUnavailable,
	UnusedImport,
	UnusedPrivateMethod,
	UnusedPrivateProperty,
	UnusedPrivateConstant,
	UnreachableCode,
	RedundantUnionMember,
	RedundantNullsafe,
	UndefinedVariable,
	UnusedVariable,
	Deprecated,
	UnusedResult,
	InvalidOverride,
	ArgumentCountMismatch,
	UnknownNamedArgument,
	UnknownColumn,
	UnknownRelation,
	UnknownView,
	UnknownComponent,
	UnclosedDirective,
}

func ParseCode(s string) (Code, error) {
	code := Code(s)
	if !slices.Contains(All, code) {
		return "", fmt.Errorf("unknown diagnostic rule %q", s)
	}
	return code, nil
}

type Severity int

const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

var Severities = []Severity{SeverityError, SeverityWarning, SeverityInfo, SeverityHint}

var severityNames = map[Severity]string{
	SeverityError:   "error",
	SeverityWarning: "warning",
	SeverityInfo:    "info",
	SeverityHint:    "hint",
}

func (s Severity) String() string {
	return severityNames[s]
}

func ParseSeverity(s string) (Severity, error) {
	for severity, name := range severityNames {
		if name == s {
			return severity, nil
		}
	}
	return 0, fmt.Errorf("unknown severity %q (want error, warning, info or hint)", s)
}

type Source string

const SourceTusk Source = "tusk-php"

type ExternalSource string

const (
	SourcePHPStan ExternalSource = "phpstan"
	SourcePint    ExternalSource = "pint"
)

var ExternalSources = []ExternalSource{SourcePHPStan, SourcePint}

const pintFixerSeparator = ", "

func PintCode(fixers []string) string {
	return strings.Join(fixers, pintFixerSeparator)
}

func parseExternalSource(s string) (ExternalSource, bool) {
	source := ExternalSource(s)
	return source, slices.Contains(ExternalSources, source)
}

func identifiers(source, code string) []string {
	if source == string(SourcePint) {
		return strings.Split(code, pintFixerSeparator)
	}
	return []string{code}
}

type Target struct {
	source string
	code   string
}

func NativeTarget(code Code) Target {
	return Target{source: string(SourceTusk), code: string(code)}
}

func AllNativeTarget() Target {
	return Target{source: string(SourceTusk)}
}

func ExternalTarget(source ExternalSource, identifier string) Target {
	return Target{source: string(source), code: identifier}
}

func TargetsOf(source, code string) ([]Target, error) {
	if external, ok := parseExternalSource(source); ok {
		ids := identifiers(source, code)
		targets := make([]Target, len(ids))
		for i, id := range ids {
			targets[i] = ExternalTarget(external, id)
		}
		return targets, nil
	}
	if Source(source) != SourceTusk {
		return nil, fmt.Errorf("unknown diagnostic source %q", source)
	}
	parsed, err := ParseCode(code)
	if err != nil {
		return nil, err
	}
	return []Target{NativeTarget(parsed)}, nil
}

func ParseTarget(s string) (Target, error) {
	if s == string(SourceTusk) {
		return AllNativeTarget(), nil
	}
	name, identifier, qualified := strings.Cut(s, ":")
	if external, ok := parseExternalSource(name); ok {
		return ExternalTarget(external, identifier), nil
	}
	if qualified {
		return Target{}, fmt.Errorf("unknown diagnostic source %q", name)
	}
	code, err := ParseCode(s)
	if err != nil {
		return Target{}, err
	}
	return NativeTarget(code), nil
}

func (t Target) Matches(source, code string) bool {
	return t.source == source && (t.code == "" || slices.Contains(identifiers(source, code), t.code))
}

func (t Target) String() string {
	switch {
	case t.code == "":
		return t.source
	case t.source == string(SourceTusk):
		return t.code
	default:
		return t.source + ":" + t.code
	}
}
