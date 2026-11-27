package container

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const smtpMailerWith = "<?php\nnamespace App\\Mail;\nuse Symfony\\Component\\DependencyInjection\\Attribute\\AsAlias;\nuse App\\Contract\\MailerInterface;\nuse App\\Contract\\Transport;\n"

func symfonyProject(t *testing.T, files map[string]string) (*ContainerAnalyzer, string) {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		writeProjectFile(t, root, rel, content)
	}
	ca := NewContainerAnalyzer(symbols.NewIndex(), root, "symfony")
	ca.Analyze()
	return ca, root
}

func resolvedConcrete(ca *ContainerAnalyzer, abstract string) string {
	if b := ca.ResolveDependency(abstract); b != nil {
		return b.Concrete
	}
	return ""
}

func TestAsAliasBindsTheNamedServiceToTheClass(t *testing.T) {
	ca, _ := symfonyProject(t, map[string]string{
		"src/Mail/ASendmailMailer.php": "<?php\nnamespace App\\Mail;\nuse App\\Contract\\MailerInterface;\nclass ASendmailMailer implements MailerInterface {}\n",
		"src/Mail/SmtpMailer.php":      smtpMailerWith + "#[AsAlias(MailerInterface::class)]\n#[AsAlias(id: 'app.mailer')]\nfinal class SmtpMailer implements MailerInterface {}\n",
	})
	for _, abstract := range []string{`App\Contract\MailerInterface`, "app.mailer"} {
		if got := resolvedConcrete(ca, abstract); got != `App\Mail\SmtpMailer` {
			t.Errorf("%s resolves to %q", abstract, got)
		}
	}
	if b := ca.LookupBinding("app.mailer"); b == nil || b.DefinitionURI == "" || b.Definition.Start.Line != 6 {
		t.Errorf("the alias points at its attribute: %+v", b)
	}
}

func TestAsAliasWithoutAnIDUsesTheOnlyInterface(t *testing.T) {
	for name, tc := range map[string]struct {
		class string
		want  string
	}{
		"no arguments":     {"#[AsAlias]\nclass SmtpMailer implements MailerInterface {}\n", `App\Mail\SmtpMailer`},
		"explicit null id": {"#[AsAlias(id: null, public: true)]\nclass SmtpMailer implements MailerInterface {}\n", `App\Mail\SmtpMailer`},
		"two interfaces":   {"#[AsAlias]\nclass SmtpMailer implements MailerInterface, Transport {}\n", ""},
		"a parent class":   {"#[AsAlias]\nclass SmtpMailer extends Base implements MailerInterface {}\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			ca, _ := symfonyProject(t, map[string]string{
				"src/Mail/ASendmailMailer.php": "<?php\nnamespace App\\Mail;\nuse App\\Contract\\MailerInterface;\nclass ASendmailMailer implements MailerInterface {}\n",
				"src/Mail/SmtpMailer.php":      smtpMailerWith + tc.class,
			})
			want := tc.want
			if want == "" {
				want = `App\Mail\ASendmailMailer`
			}
			if got := resolvedConcrete(ca, `App\Contract\MailerInterface`); got != want {
				t.Errorf("MailerInterface resolves to %q, want %q", got, want)
			}
		})
	}
}

func TestAsAliasIsOnlySymfonysAttribute(t *testing.T) {
	ca, _ := symfonyProject(t, map[string]string{
		"src/Mail/SmtpMailer.php": "<?php\nnamespace App\\Mail;\nuse App\\Attribute\\AsAlias;\n#[AsAlias('app.mailer')]\nclass SmtpMailer {}\n",
		"src/Mail/Fqn.php":        "<?php\nnamespace App\\Mail;\n#[\\Symfony\\Component\\DependencyInjection\\Attribute\\AsAlias('app.fqn')]\nclass Fqn {}\n",
	})
	if b := ca.LookupBinding("app.mailer"); b != nil {
		t.Errorf("another library's AsAlias binds nothing: %+v", b)
	}
	if got := resolvedConcrete(ca, "app.fqn"); got != `App\Mail\Fqn` {
		t.Errorf("a fully qualified AsAlias binds app.fqn to %q", got)
	}
}

func TestAsAliasYieldsToConfigButOverridesTheDefaults(t *testing.T) {
	ca, _ := symfonyProject(t, map[string]string{
		"config/services.yaml":    "services:\n    App\\Contract\\MailerInterface:\n        alias: App\\Mail\\Configured\n",
		"src/Mail/Configured.php": "<?php\nnamespace App\\Mail;\nclass Configured {}\n",
		"src/Mail/SmtpMailer.php": smtpMailerWith + "use Psr\\Log\\LoggerInterface;\n#[AsAlias(MailerInterface::class)]\n#[AsAlias(LoggerInterface::class)]\nclass SmtpMailer implements MailerInterface, LoggerInterface {}\n",
	})
	if got := resolvedConcrete(ca, `App\Contract\MailerInterface`); got != `App\Mail\Configured` {
		t.Errorf("services.yaml wins over the attribute, got %q", got)
	}
	if got := resolvedConcrete(ca, `Psr\Log\LoggerInterface`); got != `App\Mail\SmtpMailer` {
		t.Errorf("the attribute wins over the built-in default, got %q", got)
	}
}

func TestRemovingAsAliasDropsTheAlias(t *testing.T) {
	ca, root := symfonyProject(t, map[string]string{
		"src/Mail/SmtpMailer.php": smtpMailerWith + "#[AsAlias(id: 'app.mailer')]\nclass SmtpMailer {}\n",
	})
	if ca.LookupBinding("app.mailer") == nil {
		t.Fatal("the alias is bound")
	}
	path := writeProjectFile(t, root, "src/Mail/SmtpMailer.php", smtpMailerWith+"class SmtpMailer {}\n")
	if err := ca.Update(path); err != nil {
		t.Fatal(err)
	}
	if b := ca.LookupBinding("app.mailer"); b != nil {
		t.Errorf("the alias is gone with its attribute: %+v", b)
	}
}
