package lexer

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

var snippets = []string{
	"",
	"plain html, no php",
	"<?php",
	"<?php\r\n$a;",
	"<?php\r$a;",
	"<?php\t$a;",
	"<?phpx not a tag <?= $a ?>\n<b>",
	"<? short tags stay html",
	"<?php ?>\r\nafter ?>\rmore <?php ?>",
	"<?php enum Suit: string {} enum extends; enum implements; enum; enum /* c */ Kind {} Enum Foo {}",
	"<?php yield  from x(); yield /* c */ from y(); yield fromage; yield\nfrom;",
	"<?php class A { private(set) int $a; protected(set) $b; public(set) $c; private $d; public(sat) $e; }",
	"<?php function f(&$a, & ...$b) { $c = &$d; $e & $f; $g &= 1; $h && $i; fn() => &$j; }",
	"<?php $x = (int) (integer)(bool)( boolean )(float)(double)(string)(binary)(array)(object)(unset)(real)(foo)(int ;",
	"<?php $a <> $b; $a <=> $b; $a ** $b **= 2; $a ?? $b ??= $c; $a?->b; $a->class; $a -> list; A::class; ...$a;",
	"<?php $a .= 1 % 2 ^ 3 | 4 ~ 5 @ $b ! $c >>= 1 <<= 2 >> 3 << 4 !== 5 === 6 >= 7 <= 8 -- ++ -= += *= /= %= |= ^=;",
	"<?php \\Foo\\Bar::x(); Foo\\Bar; namespace\\Baz; NAMESPACE\\Qux; \\strlen(); \\ ; Foo\\ ;",
	"<?php 0 7 007 0777 08 1_000 0x1F 0X1f_ff 0b101 0B1_1 0o17 0O7 1.5 .5 1. 1e3 1E-3 1.5e+3 1_0.5 9223372036854775807 9223372036854775808 0x7FFFFFFFFFFFFFFF 0xFFFFFFFFFFFFFFFF 0b11111111111111111111111111111111111111111111111111111111111111111 01777777777777777777777 1_ 0x 0b2 1e;",
	"<?php 'a\\'b\\\\c' b'bin' B\"bin $x\" \"plain\" \"esc \\$x \\{$y}\" 'unterminated",
	"<?php \"$a $a[0] $a[-1] $a[0x1F] $a[0b1] $a[0o7] $a[key] $a[$k] $a->b $a?->c $a-> {$a->b['c']} ${a} ${a[1]} ${a . 'b'} {$a{'x'}} \\$no $\"",
	"<?php \"unterminated $a",
	"<?php \"unterminated plain",
	"<?php `ls $dir {$x} -la`; `plain`;",
	"<?php $a = <<<EOT\nline $a {$b} ${c}\n  EOT;\n$b = <<<\"Q\"\nq\nQ;\n$c = <<<'N'\nno $vars {$here}\n N;\n$d = <<< EOT\nEOT;\n$e = <<<EOT\r\n\tx\r\n\tEOT;\n$f = <<<EOT\nEOTX\nEOT;\n$g = b<<<EOT\nb\nEOT;\n$h = b<<<\nnot",
	"<?php $a = <<<EOT\nunterminated",
	"<?php $a = <<<'EOT'\nunterminated",
	"<?php $a = <<<'EOT\nbad",
	"<?php $a = <<<EOT x\n",
	"<?php $a = <<<\"EOT\nbad",
	"<?php $a = <<<",
	"<?php # hash\n// slash ?> html <?php /* block */ /** doc */ /**/ /***/ /** \n*/ #[Attr] #[\n/* unterminated",
	"<?php // ends at close ?>\n# also ?>x",
	"<?php __halt_compiler(); raw \x00 data",
	"<?php __HALT_COMPILER ( ) ; raw",
	"<?php __halt_compiler",
	"<?php __halt_compiler() ?>abc<?php echo 1;",
	"<?php __halt_compiler ?>abc<?= ) x y z",
	"<?php __halt_compiler ?>a<?php ?>b<?php x",
	"<?php __halt_compiler( ?><?php ) x",
	"<?php __halt_compiler ( ) __halt_compiler ( ) ; x y",
	"<?php __halt_compiler ( `a $b` x y",
	"<?php __halt_compiler ( \"a {$b}\" x y",
	"<?php __halt_compiler ( <<<EOT\na $b\nEOT;\n",
	"<?php __halt_compiler # c\n( /* x */ ) ?>raw",
	"<?php $a->/* c */class; $a-> # c\n list; $a?->/** d */ /* e */ // f\n fn; $a->#[x\n y; $a->/* u",
	"<?php 1._5; 1_.5; 1._; 1.e5; 1._e5; 1.5_e3; 1e_5; 1e5_; .5_; 0x_1; 0x1_; 0b_1; 08_1; 1__0;",
	"<?php \"$a[0x] $a[0b2] $a[0x_1] $a[0o8] $a[0o] $a[1_0] $a[0_1] $a[01] $a[-0] $a[-01] $a[0xg] $a[1x] $a[0x1_] $a[0b_1]\";",
	"<?php enum extendsFoo {} enum implementsX {} enum EXTENDS_ {} enum extends1 {} enum Implements {} enum extend {}",
	"<?php $$a; $ ; ${'a'}; $a = \"{$b[\"c\"]}\";",
	"<?php \x01 \x7f",
	"<?php if: endif; while: endwhile; for: endfor; foreach: endforeach; switch: endswitch; declare: enddeclare;",
	"<?php abstract final class C extends B implements I { use T { a as b; c insteadof d; } const X = 1; var $v; public static function f(): static { global $g; goto x; x: try {} catch (E) {} finally {} throw $e; return clone new self; } }",
	"<?php interface I {} trait T {} include 'a'; include_once 'b'; require 'c'; require_once 'd'; eval('1'); exit; die; print 1; echo 1; isset($a); unset($a); empty($a); list($a) = [1]; array(); callable; match(1) { default => 1 }; fn() => 1; if (1) {} elseif (1) {} else {} do {} while (0); continue; break; case; and or xor instanceof as readonly function() {};",
	"<?php __CLASS__ __DIR__ __FILE__ __FUNCTION__ __LINE__ __METHOD__ __NAMESPACE__ __PROPERTY__ __TRAIT__ __halt_compiler_not",
	"<?php ÿunicode_label $ÿ \\ÿ\\ÿ",
}

var triviaSnippets = []string{
	"<?php // comment at the end",
	"<?php function f(& // c\n $a, & # c\n ...$b) {} enum # c\n E {} yield // c\nfrom x();",
	"<?php $a & /* never closed",
	"<?php $a &",
	"<?php readonly(); function readonly() {} enum\n\tFoo {}",
}

var malformedSnippets = []string{
	"<?php \"$a[ x\"; \"$a[\"; \"$a[0\"; \"${\"; \"{$\"; `{$",
	"<?php ?",
	"<?php $a->",
}

var versionSnippets = []string{
	"<?php $a |> strlen(...); (void) f(); ( VOID )g(); $b | > $c;",
	"<?php enum E {} readonly class R { public function f(&$a) { return &$b & $c; } } $x = 0o17; \"$a[0o7]\";",
	"<?php class P { private(set) $a; } __PROPERTY__;",
}

func TestLexIsLosslessAndMatchesPHP(t *testing.T) {
	all := append(append(append(append([]string{}, snippets...), versionSnippets...), triviaSnippets...), malformedSnippets...)
	for v := token.PHP80; v <= token.Latest; v++ {
		for _, src := range all {
			var b strings.Builder
			end := uint32(0)
			for _, tok := range Lex(src, v) {
				if tok.Start != end || tok.End <= tok.Start || tok.Kind == token.Invalid {
					t.Fatalf("%s %q: token %s [%d,%d) doesn't continue at %d", v, src, tok.Kind, tok.Start, tok.End, end)
				}
				end = tok.End
				b.WriteString(tok.Text(src))
			}
			if b.String() != src {
				t.Errorf("%s: tokens of %q rebuild %q", v, src, b.String())
			}
		}
	}
	compare := append(append(append([]string{}, snippets...), versionSnippets...), triviaSnippets...)
	dir := t.TempDir()
	paths := make([]string, len(compare))
	for i, src := range compare {
		paths[i] = filepath.Join(dir, "s"+strconv.Itoa(i)+".php")
		if err := os.WriteFile(paths[i], []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, php := range references(t) {
		t.Run(php.version.String(), func(t *testing.T) {
			want := runReference(t, php, paths, true)
			for i, path := range paths {
				if got, expected := serialize(compare[i], php.version), decode(t, want[path]); got != expected {
					t.Errorf("%q: %s", compare[i], firstDifference(got, expected))
				}
			}
		})
	}
}

func TestLexCodeStartsInPHP(t *testing.T) {
	src := "class A {} ?>x"
	toks := LexCode(src, token.Latest)
	want := []token.Kind{token.Class, token.Whitespace, token.Identifier, token.Whitespace, token.LeftBrace, token.RightBrace, token.Whitespace, token.CloseTag, token.InlineHTML}
	if len(toks) != len(want) {
		t.Fatalf("got %v", toks)
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: %v, want %v", i, toks[i].Kind, k)
		}
	}
}
