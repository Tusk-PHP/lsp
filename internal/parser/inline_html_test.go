package parser

import "testing"

func TestTokenizeSkipsInlineHTML(t *testing.T) {
	src := "<html><?xml-ish text ?>\n" + // before the first open tag: HTML
		"<?php $a = 1; // comment ?>\n" + // a line comment ends at ?>
		"<script>jQuery( function($) { foo(); } );</script>\n" +
		"<p><?= $a ?></p>\n" +
		"<?php\nbar();\n"
	var idents, vars []Token
	var opens, closes int
	for _, tok := range New().Parse(src).Tokens {
		switch {
		case tok.Kind == TokenIdentifier:
			idents = append(idents, tok)
		case tok.Kind == TokenVariable:
			vars = append(vars, tok)
		case tok.Kind == TokenPHP:
			opens++
		case tok.Kind == TokenSemicolon && tok.Value == "?>":
			closes++
		}
	}
	if len(idents) != 1 || idents[0].Value != "bar" || idents[0].Line != 5 || idents[0].Column != 0 {
		t.Errorf("identifiers = %+v, want only bar() on line 5", idents)
	}
	if len(vars) != 2 || vars[1].Line != 3 {
		t.Errorf("variables = %+v, want $a on lines 1 and 3", vars)
	}
	if opens != 3 || closes != 2 {
		t.Errorf("open tags = %d, close tags = %d, want 3 and 2", opens, closes)
	}
}

func TestTokenizeWithoutOpenTagIsPHP(t *testing.T) {
	src := "class Foo { function bar() {} }"
	if got := ParseFile(src); got == nil || len(got.Classes) != 1 {
		t.Fatalf("a snippet without <?php should still parse as PHP: %+v", got)
	}
}

func TestOpenTagLen(t *testing.T) {
	cases := map[string]int{"<?php ": 5, "<?PHP\n": 5, "<?php": 5, "<?=": 3, "<? echo": 2, "<?xml": 0, "<?phpx": 0, "<p>": 0}
	for in, want := range cases {
		if got := OpenTagLen(in, 0); got != want {
			t.Errorf("OpenTagLen(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseIndexesFunctionsDeclaredInsideFunctions(t *testing.T) {
	src := `<?php
function export_wp() {
	/** Escapes CDATA. */
	function wxr_cdata( $str ) { return $str; }
	if ( ! function_exists( 'wxr_site_url' ) ) {
		function wxr_site_url() {}
	}
	$cb = function () {};
	$obj = new class { function method_of_anon() {} };
	?>
	<channel><?php echo wxr_cdata( 'x' ); ?></channel>
	<?php
}
`
	got := map[string]FunctionDef{}
	for _, fn := range New().Parse(src).Functions {
		got[fn.Name] = fn
	}
	for _, name := range []string{"export_wp", "wxr_cdata", "wxr_site_url"} {
		if _, ok := got[name]; !ok {
			t.Errorf("function %s not found; got %v", name, got)
		}
	}
	if _, ok := got["method_of_anon"]; ok {
		t.Errorf("anonymous class method indexed as a function")
	}
	if len(got) != 3 {
		t.Errorf("got %d functions, want 3: %v", len(got), got)
	}
	if got["wxr_cdata"].DocComment == "" || got["export_wp"].EndLine < 12 {
		t.Errorf("nested function lost its doc comment or outer end line: %+v / %+v", got["wxr_cdata"], got["export_wp"])
	}
}
