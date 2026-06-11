package container

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestParseConstructorAutowireAttrs_ServiceForm(t *testing.T) {
	source := `<?php
class MyService {
    public function __construct(
        #[Autowire(service: 'app.notifier')]
        private NotificationService $notifier,
        private LoggerInterface $logger,
    ) {}
}
`
	result := parseConstructorAutowireAttrs(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	aw, ok := result["$notifier"]
	if !ok {
		t.Fatal("expected $notifier in autowire map")
	}
	if aw.kind != "service" {
		t.Errorf("kind = %q, want 'service'", aw.kind)
	}
	if aw.value != "app.notifier" {
		t.Errorf("value = %q, want 'app.notifier'", aw.value)
	}
	// $logger has no Autowire attribute
	if _, ok := result["$logger"]; ok {
		t.Error("$logger should not have an Autowire attr")
	}
}

func TestParseConstructorAutowireAttrs_ParamForm(t *testing.T) {
	source := `<?php
class MyService {
    public function __construct(
        #[Autowire(param: 'app.some_param')]
        private string $param,
    ) {}
}
`
	result := parseConstructorAutowireAttrs(source)
	aw, ok := result["$param"]
	if !ok {
		t.Fatal("expected $param")
	}
	if aw.kind != "param" {
		t.Errorf("kind = %q, want 'param'", aw.kind)
	}
	if aw.value != "app.some_param" {
		t.Errorf("value = %q", aw.value)
	}
}

func TestParseConstructorAutowireAttrs_PercentParamForm(t *testing.T) {
	source := `<?php
class MyService {
    public function __construct(
        #[Autowire('%app.secret%')]
        private string $secret,
    ) {}
}
`
	result := parseConstructorAutowireAttrs(source)
	aw, ok := result["$secret"]
	if !ok {
		t.Fatal("expected $secret")
	}
	if aw.kind != "param" {
		t.Errorf("kind = %q, want 'param'", aw.kind)
	}
	if aw.value != "app.secret" {
		t.Errorf("value = %q, want 'app.secret'", aw.value)
	}
}

func TestParseConstructorAutowireAttrs_EnvForm(t *testing.T) {
	source := `<?php
class MyService {
    public function __construct(
        #[Autowire(env: 'APP_SECRET')]
        private string $appSecret,
    ) {}
}
`
	result := parseConstructorAutowireAttrs(source)
	aw, ok := result["$appSecret"]
	if !ok {
		t.Fatal("expected $appSecret")
	}
	if aw.kind != "env" {
		t.Errorf("kind = %q, want 'env'", aw.kind)
	}
	if aw.value != "APP_SECRET" {
		t.Errorf("value = %q, want 'APP_SECRET'", aw.value)
	}
}

func TestAnalyzeConstructorInjection_WithAutowire(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src", "Service")
	os.MkdirAll(srcDir, 0755)

	// Write the notifier service
	notifierPath := filepath.Join(srcDir, "NotificationService.php")
	os.WriteFile(notifierPath, []byte(`<?php
namespace App\Service;
class NotificationService {
    public function notify(string $msg): void {}
}
`), 0644)

	// Write the service with Autowire.
	// NOTE: the PHP parser may not correctly capture param names for
	// promoted constructor properties preceded by attributes — we correlate
	// Autowire info by position (index) rather than by ParamName.
	autowiredPath := filepath.Join(srcDir, "AutowiredService.php")
	autowiredSrc := `<?php
namespace App\Service;

use Symfony\Component\DependencyInjection\Attribute\Autowire;
use Psr\Log\LoggerInterface;

class AutowiredService {
    public function __construct(
        #[Autowire(service: 'app.notifier')]
        private NotificationService $notifier,
        #[Autowire(param: 'app.name')]
        private string $appName,
        #[Autowire(env: 'APP_ENV')]
        private string $appEnv,
        private LoggerInterface $logger,
    ) {}
}
`
	os.WriteFile(autowiredPath, []byte(autowiredSrc), 0644)

	idx := symbols.NewIndex()
	idx.IndexFile("file://"+filepath.ToSlash(notifierPath), mustRead(t, notifierPath))
	idx.IndexFile("file://"+filepath.ToSlash(autowiredPath), autowiredSrc)

	ca := NewContainerAnalyzer(idx, dir, "symfony")
	// Register app.notifier manually
	ca.mu.Lock()
	ca.bindings["app.notifier"] = &ServiceBinding{
		Abstract:  "app.notifier",
		Concrete:  "App\\Service\\NotificationService",
		Singleton: true,
	}
	ca.mu.Unlock()

	deps := ca.AnalyzeConstructorInjection("App\\Service\\AutowiredService")
	if len(deps) == 0 {
		t.Fatal("expected deps, index may not have parsed the class")
	}

	// Verify by Autowire position.
	// The positional slice from parseConstructorAutowireAttrsByPosition is:
	// [0] service:app.notifier, [1] param:app.name, [2] env:APP_ENV, [3] (none)
	// deps length from index may vary if parser skips attributed params, so we
	// verify the AutowireKind/Value attributes on whichever deps we got.
	var serviceDepFound, paramDepFound, envDepFound bool
	for _, dep := range deps {
		switch dep.AutowireKind {
		case "service":
			serviceDepFound = true
			if dep.AutowireValue != "app.notifier" {
				t.Errorf("service AutowireValue = %q, want 'app.notifier'", dep.AutowireValue)
			}
			if dep.ResolvedConcrete != "App\\Service\\NotificationService" {
				t.Errorf("service ResolvedConcrete = %q", dep.ResolvedConcrete)
			}
		case "param":
			paramDepFound = true
			if dep.AutowireValue != "app.name" {
				t.Errorf("param AutowireValue = %q, want 'app.name'", dep.AutowireValue)
			}
		case "env":
			envDepFound = true
			if dep.AutowireValue != "APP_ENV" {
				t.Errorf("env AutowireValue = %q, want 'APP_ENV'", dep.AutowireValue)
			}
		}
	}

	if !serviceDepFound {
		t.Error("expected a dep with AutowireKind='service'")
	}
	if !paramDepFound {
		t.Error("expected a dep with AutowireKind='param'")
	}
	if !envDepFound {
		t.Error("expected a dep with AutowireKind='env'")
	}
}

func TestAnalyzeConstructorInjection_LaravelUnaffected(t *testing.T) {
	// Autowire parsing only runs for Symfony; ensure Laravel injection still works.
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Controller.php", `<?php
namespace App;
use Illuminate\Contracts\Cache\Factory as CacheFactory;
class Controller {
    public function __construct(
        private CacheFactory $cache
    ) {}
}
`)
	ca := NewContainerAnalyzer(idx, "/tmp", "laravel")
	ca.Analyze()

	deps := ca.AnalyzeConstructorInjection("App\\Controller")
	if len(deps) == 0 {
		t.Fatal("expected at least one dep")
	}
	// AutowireKind should be empty for Laravel
	for _, dep := range deps {
		if dep.AutowireKind != "" {
			t.Errorf("expected empty AutowireKind for Laravel, got %q", dep.AutowireKind)
		}
	}
}

func TestParseAutowireAttributeLine(t *testing.T) {
	tests := []struct {
		line     string
		wantKind string
		wantVal  string
		wantNil  bool
	}{
		{"        #[Autowire(service: 'app.notifier')]", "service", "app.notifier", false},
		{"        #[Autowire(param: 'app.name')]", "param", "app.name", false},
		{"        #[Autowire(env: 'APP_ENV')]", "env", "APP_ENV", false},
		{"        #[Autowire('%app.secret%')]", "param", "app.secret", false},
		{"        #[Route('/foo')]", "", "", true},
		{"        #[Autowire]", "", "", true}, // no args
	}
	for _, tt := range tests {
		result := parseAutowireAttributeLine(tt.line)
		if tt.wantNil {
			if result != nil {
				t.Errorf("line=%q: expected nil, got %+v", tt.line, result)
			}
			continue
		}
		if result == nil {
			t.Errorf("line=%q: expected non-nil", tt.line)
			continue
		}
		if result.kind != tt.wantKind {
			t.Errorf("line=%q: kind=%q want=%q", tt.line, result.kind, tt.wantKind)
		}
		if result.value != tt.wantVal {
			t.Errorf("line=%q: value=%q want=%q", tt.line, result.value, tt.wantVal)
		}
	}
}

func TestExtractParamName(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"        private NotificationService $notifier,", "$notifier"},
		{"        private string $appName,", "$appName"},
		{"        LoggerInterface $logger", "$logger"},
		{"    ) {", ""},
	}
	for _, tt := range tests {
		got := extractParamName(tt.line)
		if got != tt.want {
			t.Errorf("line=%q: got %q, want %q", tt.line, got, tt.want)
		}
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
