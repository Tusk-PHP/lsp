package hover

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestEffectiveDocBlockSkipsEmptyDocBlocks(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///classes.php", `<?php
interface Contract {
    public function stop(): void;
    /**
     */
    public function run(): void;
}
class Base {
    public $run;
    /** Base doc. */
    public function run(): void {}
}
/**
 */
class Impl extends Base implements Missing, Contract {
    /**
     */
    public function run(): void {}
    /**
     */
    public function lonely(): void {}
    /** Own doc. */
    public function own(): void {}
}
`)
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	for fqn, want := range map[string]string{
		"Impl::run":    "Base doc.",
		"Impl::own":    "Own doc.",
		"Impl::lonely": "",
		"Impl":         "",
	} {
		sym := idx.Lookup(fqn)
		if sym == nil {
			t.Fatalf("%s is not indexed", fqn)
		}
		if doc := p.getEffectiveDocBlock(sym); doc == nil || doc.Summary != want {
			t.Errorf("effective doc block of %s = %+v, want summary %q", fqn, doc, want)
		}
	}
}
