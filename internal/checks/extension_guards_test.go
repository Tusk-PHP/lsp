package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestExtensionGuardsCover(t *testing.T) {
	g := findExtensionGuards(`<?php
if ( ! extension_loaded( 'imagick' ) || ! class_exists( 'ImagickPixel', false ) ) { return; }
if ( EXTENSION_LOADED( "ssh2" ) ) {}
if ( function_exists( 'apcu_fetch' ) && class_exists( '\Redis' ) ) {}
if ( extension_loaded( 'gd' ) ) {}
`)
	for _, name := range []string{"Imagick", "ImagickPixel", "ImagickDraw", "\\Imagick", "ssh2_connect", "SSH2_exec", "apcu_fetch", "Redis", "redis", "gd_info", "gdImage"} {
		if !g.covers(name) {
			t.Errorf("%s should be covered", name)
		}
	}
	for _, name := range []string{"Gdpr", "Imagickish", "apcu_store", "RedisArray", "ssh2connect", "Missing"} {
		if g.covers(name) {
			t.Errorf("%s should not be covered", name)
		}
	}
}

// WordPress: the Imagick editor checks extension_loaded('imagick') in one
// method and uses Imagick everywhere else.
const guardedImagickSource = `<?php
class WP_Image_Editor_Imagick {
    public static function test( $args = array() ) {
        if ( ! extension_loaded( 'imagick' ) || ! class_exists( 'Imagick', false ) ) {
            return false;
        }
        return true;
    }
    public function load() {
        $this->image = new Imagick();
        $pixel = new ImagickPixel( '#fff' );
        $formats = Imagick::queryFormats( 'WEBP' );
        if ( $e instanceof ImagickException ) {}
        $link = ssh2_connect( 'host' );
        $other = new NotAnExtensionClass();
    }
}
`

func TestUnknownSymbolsSkipGuardedExtensionNames(t *testing.T) {
	file := parser.ParseFile(guardedImagickSource)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})
	var got []string
	for _, f := range (&UnknownClassRule{}).Check(file, guardedImagickSource, idx) {
		got = append(got, f.Message)
	}
	for _, f := range (&UnknownFunctionRule{}).Check(file, guardedImagickSource, idx) {
		got = append(got, f.Message)
	}
	want := []string{"Unknown class 'NotAnExtensionClass'", "Unknown function 'ssh2_connect'"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q (ssh2 isn't guarded in this file)", got, want)
	}
}
