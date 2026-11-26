package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestRedundantNullsafeRule(t *testing.T) {
	t.Run("nullsafe on non-nullable typed parameter flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(User $user): void {
        $user?->name;
    }
}
`
		file := parser.ParseFile(source)
		checker := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line, col int, file *parser.FileNode) string {
				if expr == "$user" {
					return "User"
				}
				return ""
			},
		}
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"redundant-nullsafe"})
	})

	t.Run("nullsafe on nullable parameter not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(?User $user): void {
        $user?->name;
    }
}
`
		file := parser.ParseFile(source)
		checker := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line, col int, file *parser.FileNode) string {
				if expr == "$user" {
					return "?User"
				}
				return ""
			},
		}
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("nullsafe on mixed type not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(mixed $x): void {
        $x?->name;
    }
}
`
		file := parser.ParseFile(source)
		checker := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line, col int, file *parser.FileNode) string {
				return "mixed"
			},
		}
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("nullsafe on union with null not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(User|null $user): void {
        $user?->name;
    }
}
`
		file := parser.ParseFile(source)
		checker := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line, col int, file *parser.FileNode) string {
				return "User|null"
			},
		}
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("a variable the resolver calls null is not judged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        $x = null;
        $x?->name;
    }
}
`
		file := parser.ParseFile(source)
		checker := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line, col int, file *parser.FileNode) string {
				return "null"
			},
		}
		assertNoFindings(t, checker.Check(file, source, nil))
	})

	t.Run("no type resolver is no-op", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        $x?->name;
    }
}
`
		file := parser.ParseFile(source)
		checker := &RedundantNullsafeRule{}
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})
}

func TestRedundantUnionRule(t *testing.T) {
	checker := &RedundantUnionRule{}

	t.Run("duplicate union member flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(string|string $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "Duplicate type")
	})

	t.Run("object with class name flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(object|DateTime $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "covered by 'object'")
	})

	t.Run("mixed with string flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(mixed|string $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "covered by 'mixed'")
	})

	t.Run("parent child in union flagged with index", func(t *testing.T) {
		idx := symbols.NewIndex()
		idx.IndexFile("file:///parent.php", `<?php
class Animal {}
`)
		idx.IndexFile("file:///child.php", `<?php
class Dog extends Animal {}
`)
		source := `<?php
class Foo {
    public function bar(Animal|Dog $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, idx)
		hasFinding(t, findings, "redundant-union-member", "covered by parent 'Animal'")
	})

	t.Run("legitimate union not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(string|int $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("iterable and array flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(iterable|array $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := checker.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "covered by 'iterable'")
	})
}

func hasFinding(t *testing.T, findings []Finding, code, msgSubstr string) {
	t.Helper()
	for _, f := range findings {
		if string(f.Code) == code && (msgSubstr == "" || contains(f.Message, msgSubstr)) {
			return
		}
	}
	t.Errorf("expected finding with code=%q containing %q, got: %v", code, msgSubstr, findings)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestRedundantNullsafeStaysQuietWhenUnsure covers receivers the resolver
// types without their null: nullable parameters, defaults of null,
// reassigned parameters, closures' own parameters, properties and ternaries.
func TestRedundantNullsafeStaysQuietWhenUnsure(t *testing.T) {
	source := `<?php
class Registry {
    private ?Logger $logger = null;

    public function compute(?Logger $logger = null, ?Vote $vote = null, Logger $other = null) {
        $logger?->info('x');
        $vote?->addReason('y');
        $other?->info('z');
        $this->logger?->info('w');
    }

    public function reassigned(Logger $logger) {
        if (rand()) { $logger = null; }
        $logger?->info('x');
    }

    public function shadowed(Logger $logger) {
        $logger->info('a');
        return function (?Logger $logger) { return $logger?->info('b'); };
    }

    public function ternary($value) {
        $date = is_null($value) ? null : new \DateTime();
        return $date?->getTimestamp();
    }
}
`
	file := parser.ParseFile(source)
	checker := &RedundantNullsafeRule{
		// What the resolver gives today: the types without their null.
		TypeResolver: func(expr, source string, line, col int, file *parser.FileNode) string {
			switch expr {
			case "$logger", "$other", "$this->logger":
				return "Logger"
			case "$vote":
				return "Vote"
			case "$date":
				return "DateTime"
			}
			return ""
		},
	}
	assertNoFindings(t, checker.Check(file, source, nil))
}

func TestRedundantNullsafeOnThisAndPlainParameters(t *testing.T) {
	source := `<?php
class Service {
    public function run(Logger $logger): void {
        $this?->helper();
        $logger?->info('a');
        $logger->info('b');
    }
}
function standalone(Clock $clock) { return $clock?->now(); }
`
	file := parser.ParseFile(source)
	checker := &RedundantNullsafeRule{TypeResolver: func(string, string, int, int, *parser.FileNode) string { return "" }}
	findings := checker.Check(file, source, nil)
	var got []string
	for _, f := range findings {
		got = append(got, f.Message)
	}
	want := []string{
		"Redundant nullsafe operator: $this is never null",
		"Redundant nullsafe operator: type 'Logger' is non-nullable",
		"Redundant nullsafe operator: type 'Clock' is non-nullable",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(findings) > 0 && (findings[0].StartLine != 3 || findings[0].StartCol != 13) {
		t.Errorf("first finding at %d:%d", findings[0].StartLine, findings[0].StartCol)
	}
}
