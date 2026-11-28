package container

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func applied(contributions ...contribution) *bindingSet {
	set := newBindingSet(nil)
	for _, c := range contributions {
		c.apply(set)
	}
	set.resolveAliases()
	return set
}

func seed(ca *ContainerAnalyzer, bindings ...ServiceBinding) {
	current := ca.view.Load()
	next := &bindingSet{bindings: maps.Clone(current.bindings)}
	providerBindings(bindings).apply(next)
	ca.view.Store(next)
}

func writeProjectFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func laravelProvider(name, abstract, concrete string) string {
	return "<?php\nnamespace App\\Providers;\nclass " + name + " {\n    public function register() {\n        $this->app->bind('" + abstract + "', '" + concrete + "');\n    }\n}\n"
}

func concreteOf(ca *ContainerAnalyzer, abstract string) string {
	if b := ca.LookupBinding(abstract); b != nil {
		return b.Concrete
	}
	return ""
}

func TestUpdateIgnoresFilesTheBindingsAreNotReadFrom(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "payments", `App\Stripe`))
	model := writeProjectFile(t, root, "app/Models/User.php", "<?php\nnamespace App\\Models;\nclass User {}\n")
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "laravel")
	ca.Analyze()
	before := ca.view.Load()

	for _, path := range []string{model, filepath.Join(root, "composer.json"), filepath.Join(root, "app", "Providers")} {
		if err := ca.Update(path); err != nil {
			t.Fatalf("Update(%s): %v", path, err)
		}
		if ca.view.Load() != before {
			t.Errorf("saving %s rebuilt the container bindings", path)
		}
	}
}

func TestUpdateReplacesOnlyTheSavedProvidersBindings(t *testing.T) {
	root := t.TempDir()
	app := writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "payments", `App\Stripe`))
	writeProjectFile(t, root, "app/Providers/MailServiceProvider.php", laravelProvider("MailServiceProvider", "mailer", `App\Smtp`))
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "laravel")
	ca.Analyze()

	writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "reports", `App\Reports`))
	if err := ca.Update(app); err != nil {
		t.Fatal(err)
	}

	for abstract, want := range map[string]string{"payments": "", "reports": `App\Reports`, "mailer": `App\Smtp`, "cache": `Illuminate\Cache\CacheManager`} {
		if got := concreteOf(ca, abstract); got != want {
			t.Errorf("%s = %q, want %q", abstract, got, want)
		}
	}
}

func TestUpdateDropsTheBindingsOfADeletedProvider(t *testing.T) {
	root := t.TempDir()
	app := writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "payments", `App\Stripe`))
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "laravel")
	ca.Analyze()

	if err := os.Remove(app); err != nil {
		t.Fatal(err)
	}
	if err := ca.Update(app); err != nil {
		t.Fatal(err)
	}
	if b := ca.LookupBinding("payments"); b != nil {
		t.Errorf("the deleted provider's binding is still there: %+v", b)
	}

	ca.Analyze()
	if b := ca.LookupBinding("payments"); b != nil {
		t.Errorf("re-analysing brought the deleted provider back: %+v", b)
	}
}

func TestAnalyzeForgetsBindingsTheProjectNoLongerDeclares(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "payments", `App\Stripe`))
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "laravel")
	ca.Analyze()

	writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "reports", `App\Reports`))
	ca.Analyze()

	if b := ca.LookupBinding("payments"); b != nil {
		t.Errorf("a binding removed from its provider survives re-analysis: %+v", b)
	}
}

func TestLaterProviderWinsBeforeAndAfterAnUpdate(t *testing.T) {
	root := t.TempDir()
	first := writeProjectFile(t, root, "app/Providers/A/Provider.php", laravelProvider("Provider", "shared", `App\First`))
	writeProjectFile(t, root, "app/Providers/A.php", laravelProvider("A", "shared", `App\Second`))
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "laravel")
	ca.Analyze()
	if got := concreteOf(ca, "shared"); got != `App\Second` {
		t.Fatalf("shared = %q, want the provider read last", got)
	}

	if err := ca.Update(first); err != nil {
		t.Fatal(err)
	}
	if got := concreteOf(ca, "shared"); got != `App\Second` {
		t.Errorf("after saving the earlier provider, shared = %q", got)
	}
}

func TestSymfonyClassThatIsRenamedDropsItsOldBinding(t *testing.T) {
	root := t.TempDir()
	path := writeProjectFile(t, root, "src/Service/Mailer.php", "<?php\nnamespace App\\Service;\nuse App\\Contract\\SendsMail;\nclass Mailer implements SendsMail {}\n")
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "symfony")
	ca.Analyze()

	writeProjectFile(t, root, "src/Service/Mailer.php", "<?php\nnamespace App\\Service;\nclass SmtpMailer {}\n")
	if err := ca.Update(path); err != nil {
		t.Fatal(err)
	}

	for abstract, want := range map[string]string{`App\Service\Mailer`: "", `App\Contract\SendsMail`: "", `App\Service\SmtpMailer`: `App\Service\SmtpMailer`} {
		if got := concreteOf(ca, abstract); got != want {
			t.Errorf("%s = %q, want %q", abstract, got, want)
		}
	}
}

func TestSymfonyServicesYAMLSaveUpdatesItsServicesOnce(t *testing.T) {
	root := t.TempDir()
	path := writeProjectFile(t, root, "config/services.yaml", "services:\n    app.mailer:\n        class: App\\Mailer\n        tags: [mail]\n")
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "symfony")
	ca.Analyze()
	ca.Analyze()
	if b := ca.LookupBinding("app.mailer"); b == nil || !slices.Equal(b.Tags, []string{"mail"}) {
		t.Fatalf("app.mailer after two analyses: %+v", b)
	}

	writeProjectFile(t, root, "config/services.yaml", "services:\n    app.mailer:\n        class: App\\SmtpMailer\n        shared: false\n")
	if err := ca.Update(path); err != nil {
		t.Fatal(err)
	}
	b := ca.LookupBinding("app.mailer")
	if b == nil || b.Concrete != `App\SmtpMailer` || b.Singleton || len(b.Tags) != 0 {
		t.Errorf("app.mailer after the save: %+v", b)
	}
}

func TestSymfonyDefinitionsComeBeforeClassesInSrc(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "config/services.php", "<?php\nreturn function ($c) {\n    $c->services()->set('App\\Contract\\SendsMail', 'App\\Service\\SmtpMailer');\n};\n")
	class := writeProjectFile(t, root, "src/Service/Mailer.php", "<?php\nnamespace App\\Service;\nuse App\\Contract\\SendsMail;\nclass Mailer implements SendsMail {}\n")
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "symfony")
	ca.Analyze()
	if err := ca.Update(class); err != nil {
		t.Fatal(err)
	}
	if got := concreteOf(ca, `App\Contract\SendsMail`); got != `App\Service\SmtpMailer` {
		t.Errorf("SendsMail = %q, want the configured service", got)
	}
}

func TestCompareWalkOrderMatchesTheDirectoryWalk(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"a.php", "a/b.php", "ab/c.php", "a-b.php", "a/b/c.php", "b.php", "b.php.php"} {
		writeProjectFile(t, root, rel, "<?php\n")
	}
	var walked []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			walked = append(walked, path)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sorted := slices.Clone(walked)
	slices.Reverse(sorted)
	slices.SortFunc(sorted, compareWalkOrder)
	if !slices.Equal(sorted, walked) {
		t.Errorf("sorted %v, walked %v", sorted, walked)
	}
}

func TestSymfonyClassInTheGlobalNamespaceIsBoundByItsName(t *testing.T) {
	ca := NewContainerAnalyzer(symbols.NewIndex(), "/tmp", "symfony")
	set := applied(ca.parseSymfonyClasses("/app/src/Mailer.php", "<?php\nclass Mailer implements SendsMail {}\n"))
	for _, abstract := range []string{"Mailer", "SendsMail"} {
		if b := set.bindings[abstract]; b == nil || b.Concrete != "Mailer" {
			t.Errorf("%s: %+v", abstract, b)
		}
	}
}

func TestYAMLAndPHPConfigAliasesResolveAlike(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "config/services.yaml", "services:\n    app.smtp:\n        class: App\\Smtp\n        shared: false\n    yaml.mailer:\n        alias: app.smtp\n")
	writeProjectFile(t, root, "config/services.php", "<?php\nreturn function ($c) {\n    $c->services()->alias('php.mailer', 'app.smtp');\n};\n")
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "symfony")
	ca.Analyze()

	yaml, php := ca.LookupBinding("yaml.mailer"), ca.LookupBinding("php.mailer")
	if yaml == nil || php == nil {
		t.Fatalf("yaml %+v, php %+v", yaml, php)
	}
	if yaml.Concrete != `App\Smtp` || yaml.Concrete != php.Concrete || yaml.Singleton != php.Singleton || yaml.Alias != php.Alias {
		t.Errorf("yaml alias %+v, php alias %+v", yaml, php)
	}
}

func TestSymfonyServicesYAMLReadsOnlyServiceDefinitions(t *testing.T) {
	set := applied(parseSymfonyServicesYAML("/app/config/services.yaml", `parameters:
    locale: en
services:
    _defaults:
        autowire: true
    app.mailer:
        class: App\Mailer
        arguments:
            - '@logger'
        tags:
            - mail
            - 'kernel.reset'
        public: true
when@dev:
    parameters:
        debug: true
`))
	if len(set.bindings) != 1 {
		t.Fatalf("bindings %v, want app.mailer alone", slices.Sorted(maps.Keys(set.bindings)))
	}
	b := set.bindings["app.mailer"]
	if b == nil || b.Concrete != `App\Mailer` || !slices.Equal(b.Tags, []string{"mail", "kernel.reset"}) {
		t.Errorf("app.mailer = %+v", b)
	}
}

func TestSymfonyServicesConfigRedefinesKnownServices(t *testing.T) {
	root := t.TempDir()
	yaml := writeProjectFile(t, root, "config/services.yaml", "services:\n    app.smtp:\n        class: App\\Smtp\n        shared: false\n    Symfony\\Component\\HttpFoundation\\RequestStack:\n        tags: [kernel.reset]\n")
	writeProjectFile(t, root, "config/services.yml", "services:\n    app.smtp:\n        shared: true\n")
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "symfony")
	ca.Analyze()

	if b := ca.LookupBinding("app.smtp"); b == nil || !b.Singleton || b.Source != yaml {
		t.Errorf("app.smtp = %+v, want shared again by services.yml and defined in services.yaml", b)
	}
	stack := ca.LookupBinding(`Symfony\Component\HttpFoundation\RequestStack`)
	if stack == nil || stack.Concrete != `Symfony\Component\HttpFoundation\RequestStack` || stack.Source != yaml || stack.DefinitionURI != pathToFileURI(yaml) || stack.Definition.Start.Line != 4 || !slices.Equal(stack.Tags, []string{"kernel.reset"}) {
		t.Errorf("RequestStack = %+v, want the default service defined and tagged in services.yaml", stack)
	}
}

func TestAnalyzeReadsOnlyTheServicesConfigAndSkipsBrokenLinks(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "config/services.yaml", "services:\n    app.mailer:\n        class: App\\Mailer\n")
	writeProjectFile(t, root, "config/packages/messenger.yaml", "services:\n    app.ghost:\n        class: App\\Ghost\n")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "missing.php"), filepath.Join(root, "src", "Gone.php")); err != nil {
		t.Fatal(err)
	}
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "symfony")
	ca.Analyze()
	if ca.LookupBinding("app.mailer") == nil || ca.LookupBinding("app.ghost") != nil {
		t.Errorf("bindings %v, want app.mailer from services.yaml and nothing from config/packages", ca.GetBindings())
	}
}

func TestUpdateReportsAServicesConfigItCannotRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "services.yaml")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "symfony")
	ca.Analyze()
	before := ca.view.Load()
	err := ca.Update(path)
	if err == nil || !strings.Contains(err.Error(), "re-read container bindings from "+path) {
		t.Fatalf("Update(%s) = %v, want a read error naming it", path, err)
	}
	if ca.view.Load() != before {
		t.Error("a failed read rebuilt the container bindings")
	}
}

func TestOnlySymfonyWatchesItsServicesConfig(t *testing.T) {
	if got := NewContainerAnalyzer(symbols.NewIndex(), "/app", "symfony").WatchGlobs(); !slices.Equal(got, []string{"**/config/services.yaml", "**/config/services.yml"}) {
		t.Errorf("symfony watches %v", got)
	}
	if got := NewContainerAnalyzer(symbols.NewIndex(), "/app", "laravel").WatchGlobs(); got != nil {
		t.Errorf("laravel watches %v", got)
	}
}

func TestAStaleReadDoesNotReplaceANewerUpdate(t *testing.T) {
	root := t.TempDir()
	path := writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "payments", `App\Stripe`))
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "laravel")
	stale, err := ca.read(path, phaseLaravelProvider)
	if err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "reports", `App\Reports`))
	if err := ca.Update(path); err != nil {
		t.Fatal(err)
	}
	ca.store(path, stale)
	if ca.LookupBinding("reports") == nil || ca.LookupBinding("payments") != nil {
		t.Errorf("bindings %v, want the newer update's", ca.GetBindings())
	}
}

func TestAnalyzeKeepsAnUpdateThatFinishedWhileItRead(t *testing.T) {
	root := t.TempDir()
	path := writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "payments", `App\Stripe`))
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "laravel")
	seq := ca.seq.Add(1)
	found := ca.readAll(seq)
	writeProjectFile(t, root, "app/Providers/AppServiceProvider.php", laravelProvider("AppServiceProvider", "reports", `App\Reports`))
	if err := ca.Update(path); err != nil {
		t.Fatal(err)
	}
	ca.storeAll(seq, found)
	if ca.LookupBinding("reports") == nil || ca.LookupBinding("payments") != nil {
		t.Errorf("bindings %v, want the update's", ca.GetBindings())
	}
}
