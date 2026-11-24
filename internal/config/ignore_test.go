package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIgnoreRuleMatchesPath(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"app/Legacy/**", "app/Legacy/Old.php", true},
		{"app/Legacy/**", "app/Legacy/Deep/Older.php", true},
		{"app/Legacy/**", "app/Http/Kernel.php", false},
		{"**/*.blade.php", "resources/views/welcome.blade.php", true},
		{"**/*.blade.php", "welcome.blade.php", true},
		{"src/*.php", "src/Kernel.php", true},
		{"src/*.php", "src/Http/Kernel.php", false},
		{"/database/migrations/**", "database/migrations/2020_create_users.php", true},
		{"app/**/Legacy.php", "app/Http/Kernel.php", false},
		{"app/Http/Kernel.php", "app/Http", false},
		{"./app/Legacy/**", "app/Legacy/Old.php", true},
		{`app\\Legacy\\**`, "app/Legacy/Old.php", true},
		{"app/Legacy", "app/Legacy/Deep/Older.php", true},
		{"app/Legacy/", "app/Legacy/Old.php", true},
		{"app/Legacy", "app/LegacyBridge.php", false},
		{"*.blade.php", "resources/views/deep/welcome.blade.php", true},
		{"Legacy", "app/Legacy/Old.php", true},
		{"/helpers.php", "helpers.php", true},
		{"/helpers.php", "app/helpers.php", false},
	}
	for _, c := range cases {
		var rule IgnoreRule
		if err := json.Unmarshal([]byte(`{"path":"`+c.pattern+`"}`), &rule); err != nil {
			t.Fatalf("%s: %v", c.pattern, err)
		}
		if !rule.HasPath() {
			t.Errorf("%s: a rule with a path has one", c.pattern)
		}
		if got := rule.MatchesPath(c.path); got != c.want {
			t.Errorf("%s on %s: got %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestIgnoreRuleMatchesCodeAndMessage(t *testing.T) {
	var rule IgnoreRule
	if err := json.Unmarshal([]byte(`{"code":"unknown-member","message":"^Method .*::magic"}`), &rule); err != nil {
		t.Fatal(err)
	}
	if !rule.MatchesPath("any/file.php") || rule.HasPath() {
		t.Error("a rule without a path matches every file")
	}
	if !rule.Matches("tusk-php", "unknown-member", "Method Foo::magicCall not found") {
		t.Error("code and message match")
	}
	if rule.Matches("tusk-php", "unknown-member", "Property Foo::magic not found") || rule.Matches("phpstan", "unknown-member", "Method Foo::magicCall not found") {
		t.Error("message regex and source must both apply")
	}
	var external IgnoreRule
	if err := json.Unmarshal([]byte(`{"code":"phpstan"}`), &external); err != nil {
		t.Fatal(err)
	}
	if !external.Matches("phpstan", "argument.type", "x") || external.Matches("tusk-php", "unknown-member", "x") {
		t.Error("a tool name matches all of that tool's findings and nothing else")
	}
}

func TestIgnoreRuleRejectsInvalidEntries(t *testing.T) {
	for raw, want := range map[string]string{
		`{}`:                         "needs at least one of code, path or message",
		`[1]`:                        "diagnosticIgnore entry [1]",
		`{"message":"(unclosed"}`:    `diagnosticIgnore message "(unclosed"`,
		`{"path":"app/[x"}`:          `diagnosticIgnore path "app/[x"`,
		`{"path":"./"}`:              `diagnosticIgnore path "./": names no file or directory`,
		`{"code":"unknown-members"}`: `unknown diagnostic rule "unknown-members"`,
		`{"code":"psalm:x"}`:         `unknown diagnostic source "psalm"`,
	} {
		var rule IgnoreRule
		err := json.Unmarshal([]byte(raw), &rule)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got error %v, want one containing %q", raw, err, want)
		}
	}
}

func TestLoadFromFileReadsDiagnosticIgnore(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tusk-php.json")
	if err := os.WriteFile(path, []byte(`{"diagnosticIgnore":[{"code":"phpstan:argument.type","path":"app/Legacy/**"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rules := cfg.DiagnosticIgnore.Rules
	if len(rules) != 1 || !rules[0].Matches("phpstan", "argument.type", "") || !rules[0].MatchesPath("app/Legacy/A.php") || len(cfg.Problems()) != 0 {
		t.Errorf("got %+v", cfg.DiagnosticIgnore)
	}
	if err := os.WriteFile(path, []byte(`{"phpVersion":"8.2","diagnosticIgnore":[{"message":"("},{"code":"unknown-member"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = LoadFromFile(path)
	if err != nil {
		t.Fatalf("an invalid rule is skipped, not the whole file: %v", err)
	}
	if cfg.PHPVersion != "8.2" || len(cfg.DiagnosticIgnore.Rules) != 1 {
		t.Errorf("the rest of the file still applies: %+v", cfg)
	}
	if problems := cfg.Problems(); len(problems) != 1 || !strings.Contains(problems[0].Error(), `diagnosticIgnore message "("`) {
		t.Errorf("the skipped rule is reported: %v", problems)
	}
	if err := os.WriteFile(path, []byte(`{"diagnosticIgnore":{"code":"unknown-member"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromFile(path); err == nil || !strings.Contains(err.Error(), "diagnosticIgnore") {
		t.Errorf("diagnosticIgnore that isn't a list fails the load: %v", err)
	}
}

func TestIgnoreRuleRoundTrips(t *testing.T) {
	raw := `{"code":"unknown-member","path":"app/**","message":"^x"}`
	var rule IgnoreRule
	if err := json.Unmarshal([]byte(raw), &rule); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(rule)
	if err != nil || string(out) != raw {
		t.Errorf("got %s, %v", out, err)
	}
	rules := IgnoreRules{Rules: []IgnoreRule{rule}}
	if out, err := json.Marshal(rules); err != nil || string(out) != "["+raw+"]" {
		t.Errorf("a list of rules marshals as a list: %s, %v", out, err)
	}
}
