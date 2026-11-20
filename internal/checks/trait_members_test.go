package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestUnusedPrivateUsedByTraits(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///CompressorTrait.php", `<?php
namespace App;
trait CompressorTrait {
    use StreamTrait;
    public function compress(string $path): void { $this->compressWithBinary($path); }
    abstract private function createStreamContext();
}`)
	idx.IndexFile("file:///StreamTrait.php", `<?php
namespace App;
trait StreamTrait {
    public function open() { return $this->options; }
}`)
	src := `<?php
namespace App;
class Gzip {
    use CompressorTrait;
    private array $options = [];
    private int $level = 9;
    private function compressWithBinary(string $path): void {}
    private function createStreamContext() {}
    private function neverCalled() {}
}
class Plain {
    use MissingTrait;
    private function neverCalled() {}
}`
	idx.IndexFile("file:///Gzip.php", src)
	var got []string
	for _, f := range (&UnusedPrivateRule{}).Check(parser.ParseFile(src), src, idx) {
		got = append(got, f.Message)
	}
	want := []string{
		"Private method 'Gzip::neverCalled' is never used",
		"Private property 'Gzip::$level' is never used",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %q, want %q (Plain uses a trait the index doesn't have, so it stays quiet)", got, want)
	}
}

func TestOverrideOfParentTraitMethod(t *testing.T) {
	idx := symbols.NewIndex()
	src := `<?php
trait SetsUp { public function setUpTraits(): void {} }
trait Helpers { public function helper(): void {} }
abstract class TestCase { use SetsUp; }
class ExampleTest extends TestCase {
    use Helpers;
    #[\Override]
    public function setUpTraits(): void {}
    #[\Override]
    public function helper(): void {}
}`
	idx.IndexFile("file:///test.php", src)
	var got []string
	for _, f := range (&InvalidOverrideRule{}).Check(parser.ParseFile(src), src, idx) {
		got = append(got, f.Message)
	}
	// The parent's trait method is overridden; the class's own trait method
	// is only replaced, as PHP reports.
	if len(got) != 1 || got[0] != "'helper' has #[\\Override], but there's no such method to override" {
		t.Fatalf("got %q", got)
	}
}

// PHP 8.6: #[\Override] on a class constant or enum case must override one.
func TestOverrideOnConstants(t *testing.T) {
	idx := symbols.NewIndex()
	parentSrc := `<?php
namespace App;
interface HasLimit { const LIMIT = 1; }
class Base { public const NAME = 'base'; private const SECRET = 1; }
`
	idx.IndexFile("file:///base.php", parentSrc)
	src := `<?php
namespace App;
final class Child extends Base implements HasLimit {
    #[\Override]
    public const NAME = 'child';
    #[\Override]
    public const LIMIT = 2;
    #[\Override]
    public const SECRET = 2;
    #[\Override]
    public const name = 'x';
}
enum Size: int implements HasLimit {
    #[\Override]
    case LIMIT = 3;
    #[\Override]
    case Small = 1;
}
`
	idx.IndexFile("file:///child.php", src)
	var got []string
	for _, f := range (&InvalidOverrideRule{}).Check(parser.ParseFile(src), src, idx) {
		got = append(got, f.Message)
	}
	want := []string{
		"'SECRET' has #[\\Override], but there's no such constant to override",
		"'name' has #[\\Override], but there's no such constant to override",
		"'Small' has #[\\Override], but there's no such constant to override",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q", got)
	}
}
