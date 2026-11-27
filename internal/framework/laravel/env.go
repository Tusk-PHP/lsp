package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const (
	envFile        = ".env"
	envExampleFile = ".env.example"
	envClass       = `Illuminate\Support\Env`
)

// EnvKeys knows the keys the dotenv files in a Laravel app's root assign:
// .env, .env.example, and the .env.<environment> files Laravel loads instead
// of .env when APP_ENV names them (.env.testing, .env.dusk.local, …). It
// keeps only the keys and where they are, never their values, which may be
// secrets. The keys are read once and kept until FilesChanged reports a
// change to one of those files.
type EnvKeys struct {
	rootPath string
	keys     atomic.Pointer[func() envKeys]
}

type envKeys struct {
	keys  []EnvKey
	byKey map[string][]EnvDefinition
}

// EnvKey is a key the dotenv files assign. DefinedIn names the files that
// assign it, comma-separated, in the order Definitions returns them.
type EnvKey struct {
	Name      string
	DefinedIn string
}

// EnvDefinition is where one dotenv file assigns a key: File is the file's
// name, such as ".env", and Range the key's name on its line.
type EnvDefinition struct {
	File  string
	URI   string
	Range textpos.Range
}

func NewEnvKeys(rootPath string) *EnvKeys {
	e := &EnvKeys{rootPath: rootPath}
	e.invalidate()
	return e
}

// IsEnvFile reports whether name is a dotenv file Laravel or a developer
// reads keys from: .env, .env.example or .env.<environment>, but not the
// encrypted copies `php artisan env:encrypt` writes.
func IsEnvFile(name string) bool {
	if name == envFile {
		return true
	}
	rest, ok := strings.CutPrefix(name, envFile+".")
	return ok && rest != "" && !strings.HasSuffix(rest, ".encrypted")
}

// FilesChanged forgets the keys read when one of paths, created, changed or
// deleted, is a dotenv file in the root, or the root itself.
func (e *EnvKeys) FilesChanged(paths []string) {
	if e == nil || e.rootPath == "" {
		return
	}
	root := filepath.Clean(e.rootPath)
	for _, path := range paths {
		path = filepath.Clean(path)
		if within(root, path) || filepath.Dir(path) == root && IsEnvFile(filepath.Base(path)) {
			e.invalidate()
			return
		}
	}
}

func (e *EnvKeys) invalidate() {
	load := sync.OnceValue(e.read)
	e.keys.Store(&load)
}

func (e *EnvKeys) loaded() envKeys {
	return (*e.keys.Load())()
}

// Names returns every key, sorted.
func (e *EnvKeys) Names() []string {
	if e == nil {
		return nil
	}
	keys := e.loaded().keys
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = key.Name
	}
	return names
}

// Matching returns the keys that start with partial, ignoring case, sorted:
// completion offers them. The slice is new; the keys are shared values.
func (e *EnvKeys) Matching(partial string) []EnvKey {
	if e == nil {
		return nil
	}
	keys := e.loaded().keys
	matches := make([]EnvKey, 0, len(keys))
	for _, key := range keys {
		if len(key.Name) >= len(partial) && strings.EqualFold(key.Name[:len(partial)], partial) {
			matches = append(matches, key)
		}
	}
	return matches
}

// Definitions returns where key is assigned: .env first, then .env.example,
// then the other files by name. The slice is shared: don't modify it.
func (e *EnvKeys) Definitions(key string) []EnvDefinition {
	if e == nil {
		return nil
	}
	return e.loaded().byKey[key]
}

// Definition is where key is assigned, preferring .env, then .env.example,
// or nil.
func (e *EnvKeys) Definition(key string) *EnvDefinition {
	defs := e.Definitions(key)
	if len(defs) == 0 {
		return nil
	}
	def := defs[0]
	return &def
}

func (e *EnvKeys) read() envKeys {
	dir, err := os.ReadDir(e.rootPath)
	if err != nil {
		return envKeys{}
	}
	var files []string
	for _, entry := range dir {
		if !entry.IsDir() && IsEnvFile(entry.Name()) {
			files = append(files, entry.Name())
		}
	}
	slices.SortFunc(files, func(a, b string) int {
		if ra, rb := envFileRank(a), envFileRank(b); ra != rb {
			return ra - rb
		}
		return strings.Compare(a, b)
	})
	keys := envKeys{byKey: make(map[string][]EnvDefinition)}
	for _, name := range files {
		path := filepath.Join(e.rootPath, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		uri := symbols.PathToURI(path)
		last := make(map[string]textpos.Range)
		var order []string
		for _, entry := range config.ParseDotenv(string(data)) {
			if _, seen := last[entry.Key]; !seen {
				order = append(order, entry.Key)
			}
			last[entry.Key] = textpos.Range{
				Start: textpos.Position{Line: entry.Line, Character: entry.Column},
				End:   textpos.Position{Line: entry.Line, Character: entry.Column + len(entry.Key)},
			}
		}
		for _, key := range order {
			keys.byKey[key] = append(keys.byKey[key], EnvDefinition{File: name, URI: uri, Range: last[key]})
		}
	}
	keys.keys = make([]EnvKey, 0, len(keys.byKey))
	for name, defs := range keys.byKey {
		keys.keys = append(keys.keys, EnvKey{Name: name, DefinedIn: definedIn(defs, "")})
	}
	slices.SortFunc(keys.keys, func(a, b EnvKey) int { return strings.Compare(a.Name, b.Name) })
	return keys
}

func envFileRank(name string) int {
	switch name {
	case envFile:
		return 0
	case envExampleFile:
		return 1
	}
	return 2
}

// ReadsEnv reports whether a call reads an env key as its first argument:
// function is the fully qualified function a function call runs, or class
// and method the fully qualified class and the method of a static call.
// Those are Laravel's env() helper and Env::get() / Env::getOrFail().
func ReadsEnv(function, class, method string) bool {
	if function != "" {
		return strings.EqualFold(function, "env")
	}
	return strings.EqualFold(class, envClass) && (strings.EqualFold(method, "get") || strings.EqualFold(method, "getOrFail"))
}

// Hover describes key by the files that assign it, never by its value.
func (e *EnvKeys) Hover(key string) string {
	var b strings.Builder
	b.WriteString("**`")
	b.WriteString(key)
	b.WriteString("`** environment variable\n\n")
	defs := e.Definitions(key)
	if len(defs) == 0 {
		b.WriteString("Not set in any `.env` file of the project: `env()` returns its default unless the environment sets it.")
		return b.String()
	}
	b.WriteString("Set in ")
	b.WriteString(definedIn(defs, "`"))
	b.WriteByte('.')
	return b.String()
}

func definedIn(defs []EnvDefinition, mark string) string {
	var b strings.Builder
	for i, def := range defs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(mark)
		b.WriteString(def.File)
		b.WriteString(mark)
	}
	return b.String()
}
