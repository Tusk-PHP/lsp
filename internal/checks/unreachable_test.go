package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func TestUnreachableCodeRule(t *testing.T) {
	rule := &UnreachableCodeRule{}

	t.Run("code after return in method body", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): string {
        return "hello";
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
		if findings[0].Severity != SeverityWarning {
			t.Errorf("expected SeverityWarning, got %d", findings[0].Severity)
		}
	})

	t.Run("code after return with trailing comment", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): string {
        return "hello"; // done
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
	})

	t.Run("code after throw", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        throw new \RuntimeException("error");
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
	})

	t.Run("code after exit", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        exit(1);
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
	})

	t.Run("code after die", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        die("fatal");
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
	})

	t.Run("return inside if block does not flag code after if", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(bool $x): string {
        if ($x) {
            return "yes";
        }
        return "no";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("multi-line returned array is not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function hostCompose(array $services): array {
        return [
            'version' => '3',
            'services' => array_merge([
                'traefik' => $this->traefikService([
                    '/var/run/docker.sock:/var/run/docker.sock',
                ]),
            ], $services),
        ];
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("code after multi-line returned array is flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function hostCompose(array $services): array {
        return [
            'version' => '3',
            'services' => $services,
        ];
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
		if findings[0].StartLine != 7 {
			t.Errorf("expected unreachable code to start after returned array on line 7, got %d", findings[0].StartLine)
		}
	})

	t.Run("code after multi-line returned array with trailing comment is flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function hostCompose(array $services): array {
        return [
            'version' => '3',
        ]; // done
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
	})

	t.Run("closing brace after return not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): string {
        return "hello";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("nested blocks - return in inner block does not affect outer", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(bool $x): string {
        if ($x) {
            return "inner";
        }
        $y = "still reachable";
        return $y;
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("no method body - abstract method", func(t *testing.T) {
		source := `<?php
abstract class Foo {
    abstract public function bar(): string;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("multiple unreachable sections", func(t *testing.T) {
		source := `<?php
class Foo {
    public function a(): string {
        return "x";
        echo "dead1";
    }
    public function b(): void {
        throw new \Exception();
        echo "dead2";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code", "unreachable-code"})
	})
}

func TestUnreachableCodeRuleDoesNotFlag(t *testing.T) {
	bodies := map[string]string{
		"returned closure": `return function ($table) {
            $joins = $this->query()->joins;

            if ($joins === null) {
                return false;
            }

            return true;
        };`,
		"closure argument in a multi-line return": `return $items->map(function ($item) {
            $name = $item->name;
            return $name;
        });`,
		"closure inside a returned array": `return [
            'a' => function () {
                $b = 1;
                return $b;
            },
        ];`,
		"returned match": `return match ($x) {
            1 => 'a',
            default => 'b',
        };`,
		"braceless if":          "if ($x) return;\n        echo 'reached';",
		"braceless else":        "if ($x) echo 'a'; else return;\n        echo 'reached';",
		"throw in a ternary":    "$a = $b ? $c : throw new \\Exception();\n        echo 'reached';",
		"exit in a ternary":     "$b ? $c : exit;\n        echo 'reached';",
		"exit in a condition":   "if ($b ? $c : exit) {\n            echo 'a';\n        }\n        echo 'reached';",
		"throw after coalesce":  "$a = $b ?? throw new \\Exception();\n        echo 'reached';",
		"switch cases":          "switch ($x) {\n            case 1:\n                return 1;\n            case 2:\n                break;\n            default:\n                echo 'a';\n        }",
		"alternative syntax":    "if ($x):\n            return 1;\n        else:\n            echo 'a';\n        endif;\n        echo 'reached';",
		"goto label":            "goto done;\n        done:\n        echo 'reached';",
		"brace in a string":     "if ($x) {\n            return '}';\n        }\n        echo 'reached';",
		"keyword as a property": "$this->return;\n        echo 'reached';",
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			source := "<?php\nclass Foo {\n    public function bar() {\n        " + body + "\n    }\n}\n"
			if findings := (&UnreachableCodeRule{}).Check(parser.ParseFile(source), source, nil); len(findings) != 0 {
				t.Errorf("unexpected findings: %+v", findings)
			}
		})
	}
}

func TestUnreachableCodeRuleInsideClosures(t *testing.T) {
	cases := map[string]struct {
		body string
		line int
	}{
		"after a closure's return": {`return function () {
            return 1;
            echo 'unreachable';
        };`, 5},
		"after a returned closure": {`return function () {
            return 1;
        };
        echo 'unreachable';`, 6},
		"after a case's return": {`switch ($x) {
            case 1:
                return 1;
                echo 'unreachable';
            case 2:
                echo 'reached';
        }`, 6},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			source := "<?php\nclass Foo {\n    public function bar() {\n        " + c.body + "\n    }\n}\n"
			findings := (&UnreachableCodeRule{}).Check(parser.ParseFile(source), source, nil)
			if len(findings) != 1 || findings[0].StartLine != c.line || findings[0].EndLine != c.line {
				t.Errorf("want one finding on line %d, got %+v", c.line, findings)
			}
		})
	}
}
