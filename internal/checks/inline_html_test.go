package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A WordPress-style admin page: PHP mixed with HTML and inline JavaScript.
const inlineHTMLTemplate = `<!DOCTYPE html>
<?php
function render_page( $title ) {
	$filters = 'all';
	?>
	<h1><?php echo $title; ?></h1>
	<script>
	jQuery( function( $ ) {
		var form = $( '#export' ), url = new URL( location.href );
		_.each( form.find( 'input' ).toArray(), function( el ) { el.focus(); } );
		tinymce.init( { selector: 'textarea' } );
	} );
	</script>
	<p class="<?php echo $filters; ?>"><?php missing_helper(); ?></p>
	<?php
}
`

func TestRulesIgnoreInlineHTMLAndScripts(t *testing.T) {
	file := parser.ParseFile(inlineHTMLTemplate)
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///page.php", inlineHTMLTemplate, symbols.SourceProject)

	fns := (&UnknownFunctionRule{}).Check(file, inlineHTMLTemplate, idx)
	if len(fns) != 1 || !strings.Contains(fns[0].Message, "missing_helper") {
		t.Errorf("unknown-function should report only the PHP call missing_helper(), got %+v", fns)
	}
	for _, f := range (&UnknownClassRule{}).Check(file, inlineHTMLTemplate, idx) {
		t.Errorf("unknown-class inside HTML/JS: %+v", f)
	}
	for _, f := range (&DeprecatedFunctionRule{}).Check(file, inlineHTMLTemplate, idx) {
		t.Errorf("deprecated inside JS (_.each): %+v", f)
	}
	for _, f := range (&ArgumentCountRule{}).Check(file, inlineHTMLTemplate, idx) {
		t.Errorf("argument-count inside JS (tinymce.init): %+v", f)
	}
	for _, f := range (&UndefinedVariableRule{}).Check(file, inlineHTMLTemplate, idx) {
		t.Errorf("variable defined before ?> reported undefined after it: %+v", f)
	}
	for _, f := range (&UnusedVariableRule{}).Check(file, inlineHTMLTemplate, idx) {
		t.Errorf("variable read in a later <?php block reported unused: %+v", f)
	}
}
