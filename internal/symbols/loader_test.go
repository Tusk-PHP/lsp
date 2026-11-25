package symbols

import (
	"strings"
	"testing"
	"testing/synctest"
)

type mapLoader struct {
	files  map[string]string
	loads  []string
	loaded []string
}

func (l *mapLoader) Load(fqn string) (LoadedClass, bool) {
	l.loads = append(l.loads, fqn)
	src, ok := l.files[strings.ToLower(fqn)]
	if !ok {
		return LoadedClass{}, false
	}
	return LoadedClass{URI: "file:///" + fqn + ".php", Source: src, From: SourceVendor}, true
}

func (l *mapLoader) Loaded(class LoadedClass) { l.loaded = append(l.loaded, class.URI) }

func TestClassLoaderLoadsAClassAndItsAncestors(t *testing.T) {
	loader := &mapLoader{files: map[string]string{
		`lib\child`: "<?php namespace Lib; class Child extends Base implements Shape { use Helps; }",
		`lib\base`:  "<?php namespace Lib; abstract class Base { public function run() {} }",
		`lib\shape`: "<?php namespace Lib; interface Shape { public function area(); }",
		`lib\helps`: "<?php namespace Lib; trait Helps { public function help() {} }",
	}}
	idx := NewIndex()
	idx.SetClassLoader(loader)

	if idx.Lookup(`Lib\Child`) == nil {
		t.Fatal("a class the loader knows is found on the first lookup")
	}
	names := map[string]bool{}
	for _, m := range idx.GetClassMembers(`Lib\Child`) {
		names[m.Name] = true
	}
	for _, want := range []string{"run", "area", "help"} {
		if !names[want] {
			t.Errorf("member %s from an ancestor is missing: %v", want, names)
		}
	}
	if len(loader.loaded) != 4 {
		t.Errorf("Loaded is told about each file once: %v", loader.loaded)
	}
	if idx.Lookup(`Lib\Missing`) != nil || idx.Lookup(`lib\MISSING`) != nil {
		t.Error("an unknown class stays unknown")
	}
	missing := 0
	for _, fqn := range loader.loads {
		if strings.EqualFold(fqn, `Lib\Missing`) {
			missing++
		}
	}
	if missing != 1 {
		t.Errorf("an unknown class is asked for once, whatever its case: %d", missing)
	}
}

func TestClassLoaderStopsWhenTheIndexIsReady(t *testing.T) {
	loader := &mapLoader{files: map[string]string{`late`: "<?php class Late {}"}}
	idx := NewIndex()
	idx.SetClassLoader(loader)
	idx.MarkReady()
	if idx.Lookup("Late") != nil || len(loader.loads) != 0 {
		t.Errorf("a ready index doesn't load: %v", loader.loads)
	}
	idx.SetClassLoader(loader)
	if idx.Lookup("Late") != nil {
		t.Error("a loader set on a ready index is ignored")
	}
	idx2 := NewIndex()
	idx2.SetClassLoader(nil)
	if idx2.Lookup("Late") != nil {
		t.Error("no loader, no loading")
	}
}

func TestClassLoaderAttemptsAreBounded(t *testing.T) {
	holder := &classLoaderHolder{attempts: map[string]chan struct{}{}}
	for i := 0; i < loaderAttemptLimit; i++ {
		if _, attempt := holder.begin(strings.Repeat("x", i+1)); attempt != attemptOwned {
			t.Fatalf("attempt %d refused early", i)
		}
	}
	if _, attempt := holder.begin("one-more"); attempt != attemptRefused {
		t.Error("past the limit, nothing more is loaded")
	}
	if _, attempt := holder.begin("X"); attempt != attemptJoined {
		t.Error("a name already asked for, whatever its case, joins the first attempt")
	}
}

func TestEnsureClassIgnoresEmptyNames(t *testing.T) {
	idx := NewIndex()
	idx.SetClassLoader(&mapLoader{})
	if idx.ensureClass("::member") || idx.ensureClass("\\") {
		t.Error("no class name, nothing to load")
	}
}

func TestClassLoaderAnswersSubtypeQuestionsBeforeIndexing(t *testing.T) {
	loader := &mapLoader{files: map[string]string{
		`lib\child`: "<?php namespace Lib; class Child extends Base {}",
		`lib\base`:  "<?php namespace Lib; class Base {}",
	}}
	idx := NewIndex()
	idx.SetClassLoader(loader)
	if !idx.IsSubtypeOf(`Lib\Child`, `Lib\Base`) {
		t.Error("the child and its parent load to answer whether one extends the other")
	}
}

type blockingLoader struct {
	started chan struct{}
	release chan struct{}
}

func (l *blockingLoader) Load(fqn string) (LoadedClass, bool) {
	close(l.started)
	<-l.release
	return LoadedClass{URI: "file:///slow.php", Source: "<?php class Slow {}", From: SourceVendor}, true
}

func (l *blockingLoader) Loaded(LoadedClass) {}

func TestClassLoaderConcurrentLookupWaitsForTheLoadInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		loader := &blockingLoader{started: make(chan struct{}), release: make(chan struct{})}
		idx := NewIndex()
		idx.SetClassLoader(loader)

		go idx.Lookup("Slow")
		<-loader.started

		second := make(chan *Symbol, 1)
		go func() { second <- idx.LookupFold("slow", nil) }()
		synctest.Wait()
		select {
		case sym := <-second:
			t.Fatalf("a lookup of a class still loading returned before the load finished: %+v", sym)
		default:
		}

		close(loader.release)
		synctest.Wait()
		if sym := <-second; sym == nil || sym.FQN != "Slow" {
			t.Errorf("the waiting lookup finds the class the first one loaded: %+v", sym)
		}
	})
}

func TestDefinitionsLoadAClassNotIndexedYet(t *testing.T) {
	loader := &mapLoader{files: map[string]string{
		`lib\child`: "<?php namespace Lib; class Child extends Base {}",
		`lib\base`:  "<?php namespace Lib; class Base { public function run($a) {} }",
	}}
	idx := NewIndex()
	idx.SetClassLoader(loader)
	defs := idx.Definitions(`Lib\Child`)
	if len(defs) != 1 {
		t.Fatalf("a class the loader knows is a definition before indexing finishes: %v", defs)
	}
	names := map[string]bool{}
	for _, m := range idx.DefinitionMembers(defs[0]) {
		names[m.Name] = true
	}
	if !names["run"] {
		t.Errorf("its ancestors are loaded too: %v", names)
	}
}
