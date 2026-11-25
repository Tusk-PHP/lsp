package parser

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/phpref"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

const shapeScript = `    static $parser;
    if (!$parser) {
        if (!@include $argv[1]) { echo $path, "\tskip\n"; continue; }
        $parser = (new PhpParser\ParserFactory)->createForVersion(PhpParser\PhpVersion::fromString($argv[2]));
    }
    try { $stmts = $parser->parse($source); } catch (PhpParser\Error $e) { echo $path, "\tskip\n"; continue; }
    $lines = [];
    $span = fn($n) => $n === null ? '_' : $n->getStartFilePos() . '-' . ($n->getEndFilePos() + 1);
    $visitor = new class($lines, $span) extends PhpParser\NodeVisitorAbstract {
        public function __construct(private array &$lines, private $span) {}
        public function enterNode(PhpParser\Node $n) {
            $s = $this->span;
            $op = null; $kids = [];
            if ($n instanceof PhpParser\Node\Expr\BinaryOp) { $op = $n->getOperatorSigil(); $kids = [$n->left, $n->right]; }
            elseif ($n instanceof PhpParser\Node\Expr\AssignRef) { $op = '=&'; $kids = [$n->var, $n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\Assign) { $op = '='; $kids = [$n->var, $n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\AssignOp) { $op = (new ReflectionClass($n))->getShortName(); $kids = [$n->var, $n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\Ternary) { $op = '?:'; $kids = [$n->cond, $n->if, $n->else]; }
            elseif ($n instanceof PhpParser\Node\Expr\BooleanNot) { $op = '!'; $kids = [$n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\UnaryMinus) { $op = 'u-'; $kids = [$n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\UnaryPlus) { $op = 'u+'; $kids = [$n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\BitwiseNot) { $op = '~'; $kids = [$n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\ErrorSuppress) { $op = '@'; $kids = [$n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\PreInc) { $op = '++x'; $kids = [$n->var]; }
            elseif ($n instanceof PhpParser\Node\Expr\PreDec) { $op = '--x'; $kids = [$n->var]; }
            elseif ($n instanceof PhpParser\Node\Expr\PostInc) { $op = 'x++'; $kids = [$n->var]; }
            elseif ($n instanceof PhpParser\Node\Expr\PostDec) { $op = 'x--'; $kids = [$n->var]; }
            elseif ($n instanceof PhpParser\Node\Expr\Cast) { $op = 'cast'; $kids = [$n->expr]; }
            elseif ($n instanceof PhpParser\Node\Expr\Instanceof_) { $op = 'instanceof'; $kids = [$n->expr, $n->class]; }
            if ($op !== null) { $this->lines[] = $s($n) . ' ' . $op . ' ' . implode(' ', array_map($s, $kids)); }
            return null;
        }
    };
    $t = new PhpParser\NodeTraverser($visitor);
    $t->traverse($stmts);
    sort($lines);
    echo $path, "\t", base64_encode(implode("\n", $lines)), "\n";`

var assignOpNames = map[token.Kind]string{
	token.PlusEqual: "Plus", token.MinusEqual: "Minus", token.MulEqual: "Mul", token.DivEqual: "Div",
	token.ConcatEqual: "Concat", token.ModEqual: "Mod", token.AndEqual: "BitwiseAnd", token.OrEqual: "BitwiseOr",
	token.XorEqual: "BitwiseXor", token.ShiftLeftEqual: "ShiftLeft", token.ShiftRightEqual: "ShiftRight",
	token.CoalesceEqual: "Coalesce", token.PowEqual: "Pow",
}

var unaryNames = map[token.Kind]string{
	token.Bang: "!", token.Minus: "u-", token.Plus: "u+", token.Tilde: "~", token.At: "@", token.Increment: "++x", token.Decrement: "--x",
}

func spanOf(n ast.Expr) string {
	if n == nil {
		return "_"
	}
	b := n.Bounds()
	return fmt.Sprintf("%d-%d", b.Start, b.End)
}

func shapes(file *ast.File) string {
	var lines []string
	add := func(n ast.Expr, op string, kids ...ast.Expr) {
		parts := []string{spanOf(n), op}
		for _, k := range kids {
			parts = append(parts, spanOf(k))
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	ast.Walk(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Binary:
			op := n.Op.Text()
			if isAmpersand(n.Op) {
				op = "&"
			}
			add(n, op, n.Left, n.Right)
		case *ast.Assign:
			switch {
			case n.ByRef:
				add(n, "=&", n.Target, n.Value)
			case n.Op == token.Equals:
				add(n, "=", n.Target, n.Value)
			default:
				add(n, assignOpNames[n.Op], n.Target, n.Value)
			}
		case *ast.Ternary:
			add(n, "?:", n.Cond, n.Then, n.Else)
		case *ast.Unary:
			add(n, unaryNames[n.Op], n.Operand)
		case *ast.Postfix:
			op := "x++"
			if n.Op == token.Decrement {
				op = "x--"
			}
			add(n, op, n.Operand)
		case *ast.Cast:
			add(n, "cast", n.Operand)
		case *ast.InstanceOf:
			add(n, "instanceof", n.Operand, n.Class)
		}
		return true
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func TestOperatorTreesMatchPHPParser(t *testing.T) {
	var php *phpref.PHP
	for _, ref := range phpref.Versions(t) {
		if ref.Version >= token.PHP84 && !strings.Contains(ref.Command[0], "docker") {
			php = &ref
		}
	}
	if php == nil {
		t.Skip("needs a local PHP 8.4 or later to run nikic/php-parser")
	}
	_, here, _, _ := runtime.Caller(0)
	autoload := filepath.Join(filepath.Dir(here), "..", "..", "..", "testdata", "laravel", "vendor", "autoload.php")
	sources := phpref.ReadAll(t, phpref.Corpus(t))
	for i, src := range programs {
		sources["program "+strconv.Itoa(i)] = src
	}
	for _, version := range []token.Version{token.PHP74, token.PHP84, token.PHP85} {
		t.Run(version.String(), func(t *testing.T) { compareShapes(t, *php, version, sources, autoload) })
	}
}

func compareShapes(t *testing.T, php phpref.PHP, version token.Version, sources map[string]string, autoload string) {
	want := php.Run(t, shapeScript, sources, autoload, version.String())
	compared, mismatched := 0, 0
	for path, src := range sources {
		encoded := want[path]
		file := Parse(src, version)
		if encoded == "skip" || len(file.Errors) > 0 || !token.Attribute.In(version) && strings.Contains(src, "#[") {
			continue
		}
		compared++
		expected := decodeShape(t, encoded)
		if got := shapes(file); got != expected {
			mismatched++
			if mismatched <= 5 {
				t.Errorf("%s:\n%s", path, firstLineDiff(src, got, expected))
			}
		}
	}
	if compared < 100 {
		t.Skip("nikic/php-parser isn't installed in the Laravel fixture")
	}
	if mismatched > 0 {
		t.Errorf("%d of %d files have operator trees that differ from nikic/php-parser on PHP %s", mismatched, compared, version)
	}
}

func decodeShape(t *testing.T, encoded string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	return string(data)
}

func firstLineDiff(src, got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	gs, ws := map[string]bool{}, map[string]bool{}
	for _, l := range g {
		gs[l] = true
	}
	for _, l := range w {
		ws[l] = true
	}
	var out []string
	for _, l := range w {
		if !gs[l] && len(out) < 3 {
			out = append(out, "  nikic only: "+l+"  "+excerpt(src, l))
		}
	}
	for _, l := range g {
		if !ws[l] && len(out) < 6 {
			out = append(out, "  ours only:  "+l+"  "+excerpt(src, l))
		}
	}
	return strings.Join(out, "\n")
}

func excerpt(src, line string) string {
	var start, end int
	if _, err := fmt.Sscanf(line, "%d-%d", &start, &end); err != nil || end > len(src) || start > end {
		return ""
	}
	text := src[start:end]
	if len(text) > 70 {
		text = text[:70] + "…"
	}
	return fmt.Sprintf("%q", text)
}
