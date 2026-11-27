package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
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
	names  []string
	byKey  map[string][]EnvDefinition
	detail map[string]string
}

// EnvDefinition is where one dotenv file assigns a key: File is the file's
// name, such as ".env".
type EnvDefinition struct {
	File     string
	Location protocol.Location
}

// EnvKeyArgument is the env key the cursor is in: Partial is the text before
// the cursor and Range the key's text between the quotes.
type EnvKeyArgument struct {
	Key     string
	Partial string
	Range   protocol.Range
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

// Names returns every key, sorted. The slice is shared: don't modify it.
func (e *EnvKeys) Names() []string {
	if e == nil {
		return nil
	}
	return e.loaded().names
}

// Definitions returns where key is assigned: .env first, then .env.example,
// then the other files by name. The slice is shared: don't modify it.
func (e *EnvKeys) Definitions(key string) []EnvDefinition {
	if e == nil {
		return nil
	}
	return e.loaded().byKey[key]
}

// Definition is where key is assigned, preferring .env, then .env.example.
func (e *EnvKeys) Definition(key string) *protocol.Location {
	defs := e.Definitions(key)
	if len(defs) == 0 {
		return nil
	}
	loc := defs[0].Location
	return &loc
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
		last := make(map[string]protocol.Range)
		var order []string
		for _, entry := range config.ParseDotenv(string(data)) {
			if _, seen := last[entry.Key]; !seen {
				order = append(order, entry.Key)
			}
			last[entry.Key] = protocol.Range{
				Start: protocol.Position{Line: entry.Line, Character: entry.Column},
				End:   protocol.Position{Line: entry.Line, Character: entry.Column + len(entry.Key)},
			}
		}
		for _, key := range order {
			if _, known := keys.byKey[key]; !known {
				keys.names = append(keys.names, key)
			}
			keys.byKey[key] = append(keys.byKey[key], EnvDefinition{File: name, Location: protocol.Location{URI: uri, Range: last[key]}})
		}
	}
	slices.Sort(keys.names)
	keys.detail = make(map[string]string, len(keys.names))
	for _, key := range keys.names {
		keys.detail[key] = definedIn(keys.byKey[key], "")
	}
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

// EnvKeyAt returns the env key the cursor is in: the first argument of
// Laravel's env() helper or of Env::get() / Env::getOrFail().
func EnvKeyAt(index *symbols.Index, doc *sourcectx.Document, pos protocol.Position) (EnvKeyArgument, bool) {
	arg, ok := doc.StringArgumentAt(pos)
	if !ok || arg.Argument != 0 || !readsEnv(index, arg) {
		return EnvKeyArgument{}, false
	}
	return EnvKeyArgument{Key: arg.Text, Partial: arg.Partial, Range: arg.Content}, true
}

func readsEnv(index *symbols.Index, arg sourcectx.StringArgument) bool {
	callee := arg.Callee
	if callee.FunctionCall {
		return strings.EqualFold(resolve.CalledFunctionFQN(index, callee.SymbolText, callee.Namespace, callee.Uses), "env")
	}
	if arg.Class == "" {
		return false
	}
	method := strings.ToLower(callee.SymbolText)
	if method != "get" && method != "getorfail" {
		return false
	}
	return strings.EqualFold(resolve.ImportedName(arg.Class, callee.Namespace, callee.Uses, "class"), envClass)
}

// Complete lists the keys starting with what is typed before the cursor,
// each replacing that text.
func (e *EnvKeys) Complete(arg EnvKeyArgument) []protocol.CompletionItem {
	if e == nil {
		return nil
	}
	keys := e.loaded()
	typed := protocol.Range{Start: arg.Range.Start, End: protocol.Position{Line: arg.Range.Start.Line, Character: arg.Range.Start.Character + len(arg.Partial)}}
	items := make([]protocol.CompletionItem, 0, len(keys.names))
	for _, key := range keys.names {
		if len(key) < len(arg.Partial) || !strings.EqualFold(key[:len(arg.Partial)], arg.Partial) {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:    key,
			Kind:     protocol.CompletionItemKindConstant,
			Detail:   keys.detail[key],
			SortText: "0" + key,
			TextEdit: &protocol.TextEdit{Range: typed, NewText: key},
		})
	}
	return items
}

// Hover describes key by the files that assign it, never by its value.
func (e *EnvKeys) Hover(arg EnvKeyArgument) string {
	var b strings.Builder
	b.WriteString("**`")
	b.WriteString(arg.Key)
	b.WriteString("`** environment variable\n\n")
	defs := e.Definitions(arg.Key)
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
