package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Laravel's broadcaster uses the Ably and Pusher SDKs, which it only
// suggests, and guards a newer Predis command with class_exists(X::class).
const optionalPackagesSource = `<?php
namespace App;

use Ably\AblyRest;
use Pusher\Pusher;
use Predis\Command\ServerFlushDatabase;
use Predis\Command\Missing;

class Manager {
    public function make() {
        $ably = new AblyRest([]);
        $pusher = new Pusher('k', 's', 'i');
        $command = class_exists(ServerFlushDatabase::class) ? ServerFlushDatabase::class : 'flushdb';
        $old = new Missing();
        $typo = new Pushr();
    }
}
`

func TestUnknownClassSkipsOptionalPackagesAndClassConstGuards(t *testing.T) {
	file := parser.ParseFile(optionalPackagesSource, token.Latest)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.2"})
	idx.SetOptionalNamespaces([]string{`ably\`, `pusher\`})
	var got []string
	for _, f := range (&UnknownClassRule{}).Check(file, optionalPackagesSource, idx) {
		got = append(got, f.Message)
	}
	want := []string{"Unknown class 'Missing'", "Unknown class 'Pushr'"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}
