package analyzer

import "testing"

// Calling a closure held in a variable shows its signature, and for PHP 8.6's
// partial application that is the parameters the placeholders leave open.
func TestSignatureHelpOfACalledClosure(t *testing.T) {
	src := `<?php
namespace App;
function greet(string $greeting, string $name, int $times = 1): string { return ''; }
class Mailer { public function send(string $to, string $subject, bool $html = false): bool { return true; } }
function run(Mailer $mailer) {
    $hello = greet('Hello', ?);
    $hello(
    $rest = greet('Hi', ...);
    $rest('Ann', 
    $byName = greet(?, ?, times: 2);
    $byName(
    $toAnn = $mailer->send('ann@example.com', ...);
    $toAnn(
    $whole = greet(...);
    $whole(
    $literal = fn (int $x, $y): int => $x;
    $literal(
    $mailer->$hello(
    $none = fn (): int => 1;
    $none(1, 
    $hello('Ann', 'extra', 
    $notAClosure = 3;
    $notAClosure(
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///run.php": src})
	help := func(needle string) (string, int) {
		t.Helper()
		p := positionOf(t, src, needle)
		p.Character += len(needle)
		h := a.GetSignatureHelp("file:///run.php", src, p)
		if h == nil {
			return "", -1
		}
		return h.Signatures[0].Label, h.ActiveParameter
	}
	for needle, want := range map[string]string{
		"$hello(":   "$hello(string $name): string",
		"$byName(":  "$byName(string $greeting, string $name): string",
		"$toAnn(":   "$toAnn(string $subject, bool $html): bool",
		"$whole(":   "$whole(string $greeting, string $name, int $times): string",
		"$literal(": "$literal(int $x, $y): int",
	} {
		if got, active := help(needle); got != want || active != 0 {
			t.Errorf("%s: got %q (active %d), want %q (active 0)", needle, got, active, want)
		}
	}
	if got, active := help("$rest('Ann', "); got != "$rest(string $name, int $times): string" || active != 1 {
		t.Errorf("second argument of a partial application's closure: got %q, active %d", got, active)
	}
	// The active parameter stays within the closure's parameters.
	if got, active := help("$none(1, "); got != "$none(): int" || active != 0 {
		t.Errorf("a closure without parameters: got %q, active %d", got, active)
	}
	if got, active := help("$hello('Ann', 'extra', "); got != "$hello(string $name): string" || active != 0 {
		t.Errorf("an argument past the last parameter: got %q, active %d", got, active)
	}
	for _, needle := range []string{"$mailer->$hello(", "$notAClosure("} {
		if got, _ := help(needle); got != "" {
			t.Errorf("%s: want no signature, got %q", needle, got)
		}
	}
}
