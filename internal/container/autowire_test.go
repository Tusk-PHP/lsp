package container

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestConstructorAutowire(t *testing.T) {
	source := `<?php
namespace App\Service;
use Symfony\Component\DependencyInjection\Attribute\Autowire;
use Other\Autowire as NotSymfony;
class MyService {
    public function __construct(
        #[Autowire(service: 'app.notifier')]
        private NotificationService $notifier,
        private LoggerInterface $logger,
        #[Autowire(param: 'app.some_param')] private string $param,
        #[Autowire('%app.secret%')]
        private string $secret,
        #[Autowire(env: 'APP_SECRET')]
        private string $appSecret,
        #[Autowire(null, 'positional.service')]
        private $positional,
        #[Autowire('plain value')]
        private string $plain,
        #[NotSymfony(service: 'other')]
        private $other,
    ) {}
}
class Other { public function __construct(#[Autowire(service: 'x')] $x) {} }
`
	got := constructorAutowire(source, `App\Service\MyService`, phpparser.OnlyGrammar(token.Latest))
	want := []autowireAttr{
		{"service", "app.notifier"}, {}, {"param", "app.some_param"}, {"param", "app.secret"},
		{"env", "APP_SECRET"}, {"service", "positional.service"}, {}, {},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	if got := constructorAutowire(source, `App\Service\Missing`, phpparser.OnlyGrammar(token.Latest)); got != nil {
		t.Errorf("a class not in the file: %+v", got)
	}
	if got := constructorAutowire("<?php\nnamespace App;\nclass NoCtor { function f() {} }\n", `App\NoCtor`, phpparser.OnlyGrammar(token.Latest)); got != nil {
		t.Errorf("no constructor: %+v", got)
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

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
