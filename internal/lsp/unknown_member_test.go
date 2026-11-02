package lsp

import "testing"

// The rule is only as good as the types the real resolver answers with, so these
// go through the server rather than a stubbed resolver.
func TestUnknownMemberThroughServer(t *testing.T) {
	cases := []phpVersionCase{
		{name: "missing member on a parameter in a namespace", diag: []string{"unknown-member"}, src: `
function run(Logger $log): void { $log->missing(); }`},
		{name: "existing member on a parameter in a namespace", noDiag: []string{"unknown-member"}, src: `
function run(Logger $log): void { $log->info('hi'); }`},
		{name: "missing member on a local in a namespace", diag: []string{"unknown-member"}, src: `
function run(): void { $log = new Logger(); $log->missing(); }`},
		{name: "union parameter with the member on one side", noDiag: []string{"unknown-member"}, src: `
function run(Logger|Person $who): void { $who->age(); $who->info('hi'); }`},
		{name: "union parameter with the member on neither side", diag: []string{"unknown-member"}, src: `
function run(Logger|Person $who): void { $who->missing(); }`},
		{name: "promoted property", noDiag: []string{"unknown-member"}, src: `
class Holder { public function __construct(private Logger $log) {} public function run(): void { $this->log->info('hi'); } }`},
		{name: "class name", noDiag: []string{"unknown-member"}, src: `
function run(): string { return Logger::class; }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runPHPVersionCase(t, tc) })
	}
}
