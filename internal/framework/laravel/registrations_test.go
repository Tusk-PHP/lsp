package laravel

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const testProvider = `<?php

namespace App\Providers;

use App\View\Components\Alert;
use Illuminate\Support\Facades\Blade;
use Illuminate\Support\Facades\View;
use Illuminate\Support\ServiceProvider;

class AppServiceProvider extends ServiceProvider
{
    public function boot(): void
    {
        Blade::directive('money', fn (string $e) => "<?php echo money($e); ?>");
        Blade::if('admin', fn () => auth()->user()?->isAdmin());
        Blade::component('notice', Alert::class);
        Blade::componentNamespace('Nightshade\\Views\\Components', 'nightshade');
        Blade::anonymousComponentPath(__DIR__.'/../../resources/views/ui', 'ui');
        $this->loadViewsFrom(dirname(__DIR__, 2).'/packages/courier/views', 'courier');
        View::addNamespace('admin', resource_path('admin-views'));
    }
}
`

func providersFixture(t *testing.T) (root string, p *Providers) {
	t.Helper()
	root = t.TempDir()
	writeFiles(t, root, map[string]string{
		"composer.json":                                         `{"autoload": {"psr-4": {"App\\": "app/"}}}`,
		"app/Providers/AppServiceProvider.php":                  testProvider,
		"resources/views/ui/button.blade.php":                   "@props(['size' => 'md'])",
		"packages/courier/views/mail/welcome.blade.php":         "",
		"packages/courier/views/components/badge.blade.php":     "",
		"resources/views/vendor/courier/mail/welcome.blade.php": "published",
		"admin-views/x.blade.php":                               "",
		"resources/admin-views/dashboard.blade.php":             "",
	})
	p = NewProviders(root, nil)
	p.regs = p.read()
	return root, p
}

func TestProviderRegistrations(t *testing.T) {
	root, p := providersFixture(t)
	r := p.regs
	keys := func(m map[string]Registration) string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	if keys(r.Directives) != "money" || keys(r.Conditionals) != "admin" {
		t.Errorf("directives %q, conditionals %q", keys(r.Directives), keys(r.Conditionals))
	}
	if r.ComponentAliases["notice"] != `App\View\Components\Alert` {
		t.Errorf("alias: %v", r.ComponentAliases)
	}
	if r.ComponentNamespaces["nightshade"] != `Nightshade\Views\Components` {
		t.Errorf("component namespace: %v", r.ComponentNamespaces)
	}
	if len(r.AnonymousPaths) != 1 || r.AnonymousPaths[0].Prefix != "ui" || r.AnonymousPaths[0].Dir != filepath.Join(root, "resources", "views", "ui") {
		t.Errorf("anonymous paths: %+v", r.AnonymousPaths)
	}
	if got := r.ViewNamespaces["courier"]; len(got) != 1 || got[0] != filepath.Join(root, "packages", "courier", "views") {
		t.Errorf("courier views: %v", got)
	}
	if got := r.ViewNamespaces["admin"]; len(got) != 1 || got[0] != filepath.Join(root, "resources", "admin-views") {
		t.Errorf("admin views: %v", got)
	}
	if r.UnknownComponents || len(r.UnknownNamespaces) != 0 {
		t.Errorf("everything here can be read: %+v %v", r.UnknownComponents, r.UnknownNamespaces)
	}
	if c := r.Custom(); !c.Directives["money"] || !c.Conditionals["admin"] || c.Where["money"].Line != 13 {
		t.Errorf("custom: %+v", c)
	}
}

func TestNamespacedComponentsAndViews(t *testing.T) {
	root, p := providersFixture(t)
	read := func(uri string) string {
		data, _ := os.ReadFile(symbols.URIToPath(uri))
		return string(data)
	}
	c := NewComponents(root, nil, p, read)
	if comp, ok := c.Component("ui::button"); !ok || len(comp.Attributes) != 1 || comp.Attributes[0].Name != "size" {
		t.Errorf("ui::button from an anonymous component path: %+v %v", comp, ok)
	}
	if _, ok := c.Component("courier::badge"); !ok {
		t.Error("courier::badge from the view namespace's components/")
	}
	if _, ok := c.Component("ui::missing"); ok {
		t.Error("ui::missing found")
	}
	if names := strings.Join(c.ComponentNames("ui::"), " "); names != "ui::button" {
		t.Errorf("ui:: names: %q", names)
	}

	v := NewViews(root)
	v.SetProviders(p)
	uri, ok := v.ViewURI("courier::mail.welcome")
	if !ok || !strings.HasSuffix(uri, "resources/views/vendor/courier/mail/welcome.blade.php") {
		t.Errorf("the published copy wins: %q %v", uri, ok)
	}
	if _, ok := v.ViewURI("admin::dashboard"); !ok {
		t.Error("admin::dashboard from View::addNamespace")
	}
	if !v.NamespaceKnown("courier") || v.NamespaceKnown("mail") {
		t.Error("NamespaceKnown")
	}
	if names := strings.Join(v.ViewNames("courier::"), " "); names != "courier::components.badge courier::mail.welcome" {
		t.Errorf("courier:: names: %q", names)
	}
}
