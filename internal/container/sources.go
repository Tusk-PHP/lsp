package container

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

type sourcePhase uint8

const (
	phaseLaravelProvider sourcePhase = iota
	phaseSymfonyServicesYAML
	phaseSymfonyServicesPHP
	phaseSymfonyClass
)

type contribution interface {
	apply(set *bindingSet)
}

type source struct {
	seq          uint64
	phase        sourcePhase
	contribution contribution
}

// Analyze reads every file the container bindings come from and replaces
// what the analyzer knows with what it found.
func (ca *ContainerAnalyzer) Analyze() {
	seq := ca.seq.Add(1)
	found := make(map[string]source)
	for _, dir := range ca.sourceDirs() {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			phase, ok := ca.phaseOf(path)
			if !ok {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			found[path] = source{seq: seq, phase: phase, contribution: ca.parse(phase, path, string(content))}
			return nil
		})
	}
	ca.sourcesMu.Lock()
	defer ca.sourcesMu.Unlock()
	for path, src := range ca.sources {
		if _, ok := found[path]; !ok && src.seq < seq {
			delete(ca.sources, path)
		}
	}
	for path, src := range found {
		if current, ok := ca.sources[path]; ok && current.seq > seq {
			continue
		}
		ca.sources[path] = src
	}
	ca.view.Store(ca.rebuild())
}

// Update re-reads one file after it changed on disk and replaces only that
// file's contribution to the bindings. A file the bindings are not read from
// is ignored; a file that no longer exists drops its contribution.
func (ca *ContainerAnalyzer) Update(path string) error {
	path = filepath.Clean(path)
	phase, ok := ca.phaseOf(path)
	if !ok {
		return nil
	}
	src := source{seq: ca.seq.Add(1), phase: phase}
	content, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Errorf("re-read container bindings from %s: %w", path, err)
	default:
		src.contribution = ca.parse(phase, path, string(content))
	}
	ca.sourcesMu.Lock()
	defer ca.sourcesMu.Unlock()
	if current, ok := ca.sources[path]; ok && current.seq > src.seq {
		return nil
	}
	ca.sources[path] = src
	ca.view.Store(ca.rebuild())
	return nil
}

func (ca *ContainerAnalyzer) sourceDirs() []string {
	switch ca.framework {
	case "laravel":
		return []string{ca.laravelProvidersDir()}
	case "symfony":
		return []string{ca.symfonyConfigDir(), ca.symfonySrcDir()}
	}
	return nil
}

func (ca *ContainerAnalyzer) laravelProvidersDir() string {
	return filepath.Join(ca.rootPath, "app", "Providers")
}

func (ca *ContainerAnalyzer) symfonyConfigDir() string {
	return filepath.Join(ca.rootPath, "config")
}

func (ca *ContainerAnalyzer) symfonySrcDir() string {
	return filepath.Join(ca.rootPath, "src")
}

var symfonyServicesYAMLFiles = []string{"services.yaml", "services.yml"}

func (ca *ContainerAnalyzer) phaseOf(path string) (sourcePhase, bool) {
	php := filepath.Ext(path) == ".php"
	switch ca.framework {
	case "laravel":
		if php && within(ca.laravelProvidersDir(), path) {
			return phaseLaravelProvider, true
		}
	case "symfony":
		configDir := ca.symfonyConfigDir()
		switch {
		case filepath.Dir(path) == configDir && slices.Contains(symfonyServicesYAMLFiles, filepath.Base(path)):
			return phaseSymfonyServicesYAML, true
		case php && within(configDir, path):
			return phaseSymfonyServicesPHP, true
		case php && within(ca.symfonySrcDir(), path):
			return phaseSymfonyClass, true
		}
	}
	return 0, false
}

func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (ca *ContainerAnalyzer) parse(phase sourcePhase, path, content string) contribution {
	switch phase {
	case phaseLaravelProvider:
		return ca.parseLaravelProvider(path, content)
	case phaseSymfonyServicesYAML:
		return parseSymfonyServicesYAML(path, content)
	case phaseSymfonyServicesPHP:
		return ca.parseSymfonyPHPConfigFile(path, content)
	case phaseSymfonyClass:
		return ca.parseSymfonyClasses(path, content)
	}
	panic(fmt.Sprintf("container: no reader for source phase %d of %s", phase, path))
}

func (ca *ContainerAnalyzer) rebuild() *bindingSet {
	set := newBindingSet(defaultBindings(ca.framework))
	paths := make([]string, 0, len(ca.sources))
	for path, src := range ca.sources {
		if src.contribution != nil {
			paths = append(paths, path)
		}
	}
	slices.SortFunc(paths, func(a, b string) int {
		return cmp.Or(cmp.Compare(ca.sources[a].phase, ca.sources[b].phase), compareWalkOrder(a, b))
	})
	for _, path := range paths {
		ca.sources[path].contribution.apply(set)
	}
	set.resolveAliases()
	return set
}

func compareWalkOrder(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		switch {
		case a[i] == b[i]:
			continue
		case a[i] == filepath.Separator:
			return -1
		case b[i] == filepath.Separator:
			return 1
		}
		return cmp.Compare(a[i], b[i])
	}
	return cmp.Compare(len(a), len(b))
}

type bindingSet struct {
	bindings map[string]*ServiceBinding
	aliases  map[string]string
}

func newBindingSet(defaults []ServiceBinding) *bindingSet {
	set := &bindingSet{bindings: make(map[string]*ServiceBinding, len(defaults)), aliases: make(map[string]string)}
	for _, b := range defaults {
		set.bindings[b.Abstract] = &b
	}
	return set
}

func (set *bindingSet) resolve(name string, seen map[string]bool) *ServiceBinding {
	if seen[name] {
		return nil
	}
	seen[name] = true
	if abstract, ok := set.aliases[name]; ok {
		if binding := set.resolve(abstract, seen); binding != nil {
			return binding
		}
	}
	binding, ok := set.bindings[name]
	if !ok {
		return nil
	}
	if binding.Alias != "" {
		if resolved := set.resolve(binding.Alias, seen); resolved != nil {
			return resolved
		}
	}
	return binding
}

func (set *bindingSet) resolveAliases() {
	for serviceID, binding := range set.bindings {
		if binding.Alias == "" || binding.Concrete != binding.Alias {
			continue
		}
		if resolved := set.resolve(serviceID, map[string]bool{}); resolved != nil {
			binding.Concrete = resolved.Concrete
			binding.Singleton = resolved.Singleton
		}
	}
}

func (set *bindingSet) ensure(serviceID, sourcePath string, pos textpos.Position) *ServiceBinding {
	definition := textpos.Range{
		Start: pos,
		End:   textpos.Position{Line: pos.Line, Character: pos.Character + len(serviceID)},
	}
	if binding, ok := set.bindings[serviceID]; ok {
		if binding.DefinitionURI == "" {
			binding.DefinitionURI = pathToFileURI(sourcePath)
			binding.Definition = definition
		}
		if binding.Source == "" {
			binding.Source = sourcePath
		}
		return binding
	}
	binding := &ServiceBinding{
		Abstract:      serviceID,
		Concrete:      serviceID,
		Singleton:     true,
		Source:        sourcePath,
		DefinitionURI: pathToFileURI(sourcePath),
		Definition:    definition,
	}
	set.bindings[serviceID] = binding
	return binding
}

type providerBindings []ServiceBinding

func (p providerBindings) apply(set *bindingSet) {
	for _, b := range p {
		set.bindings[b.Abstract] = &b
	}
}

type classBindings []ServiceBinding

func (c classBindings) apply(set *bindingSet) {
	for _, b := range c {
		if _, exists := set.bindings[b.Abstract]; !exists {
			set.bindings[b.Abstract] = &b
		}
	}
}

type sharing uint8

const (
	sharingKept sharing = iota
	sharingShared
	sharingUnshared
)

type serviceDefinition struct {
	id      string
	source  string
	at      textpos.Position
	class   string
	alias   string
	sharing sharing
	tags    []string
}

type serviceDefinitions []serviceDefinition

func (defs serviceDefinitions) apply(set *bindingSet) {
	for _, d := range defs {
		b := set.ensure(d.id, d.source, d.at)
		if d.class != "" {
			b.Concrete = d.class
		}
		if d.alias != "" {
			b.Alias = d.alias
			set.aliases[d.id] = d.alias
		}
		switch d.sharing {
		case sharingShared:
			b.Singleton = true
		case sharingUnshared:
			b.Singleton = false
		}
		b.Tags = append(b.Tags, d.tags...)
	}
}
