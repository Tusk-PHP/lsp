package laravel

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// defaultAliases are the aliases Laravel registers for every app
// (Facade::defaultAliases()): the facades by their short name, plus a few
// support classes.
var defaultAliases = map[string]string{
	"App":          `Illuminate\Support\Facades\App`,
	"Arr":          `Illuminate\Support\Arr`,
	"Artisan":      `Illuminate\Support\Facades\Artisan`,
	"Auth":         `Illuminate\Support\Facades\Auth`,
	"Benchmark":    `Illuminate\Support\Benchmark`,
	"Blade":        `Illuminate\Support\Facades\Blade`,
	"Broadcast":    `Illuminate\Support\Facades\Broadcast`,
	"Bus":          `Illuminate\Support\Facades\Bus`,
	"Cache":        `Illuminate\Support\Facades\Cache`,
	"Concurrency":  `Illuminate\Support\Facades\Concurrency`,
	"Config":       `Illuminate\Support\Facades\Config`,
	"Context":      `Illuminate\Support\Facades\Context`,
	"Cookie":       `Illuminate\Support\Facades\Cookie`,
	"Crypt":        `Illuminate\Support\Facades\Crypt`,
	"Date":         `Illuminate\Support\Facades\Date`,
	"DB":           `Illuminate\Support\Facades\DB`,
	"Eloquent":     `Illuminate\Database\Eloquent\Model`,
	"Event":        `Illuminate\Support\Facades\Event`,
	"File":         `Illuminate\Support\Facades\File`,
	"Gate":         `Illuminate\Support\Facades\Gate`,
	"Hash":         `Illuminate\Support\Facades\Hash`,
	"Http":         `Illuminate\Support\Facades\Http`,
	"Js":           `Illuminate\Support\Js`,
	"Lang":         `Illuminate\Support\Facades\Lang`,
	"Log":          `Illuminate\Support\Facades\Log`,
	"Mail":         `Illuminate\Support\Facades\Mail`,
	"Notification": `Illuminate\Support\Facades\Notification`,
	"Number":       `Illuminate\Support\Number`,
	"Password":     `Illuminate\Support\Facades\Password`,
	"Process":      `Illuminate\Support\Facades\Process`,
	"Queue":        `Illuminate\Support\Facades\Queue`,
	"RateLimiter":  `Illuminate\Support\Facades\RateLimiter`,
	"Redirect":     `Illuminate\Support\Facades\Redirect`,
	"Request":      `Illuminate\Support\Facades\Request`,
	"Response":     `Illuminate\Support\Facades\Response`,
	"Route":        `Illuminate\Support\Facades\Route`,
	"Schedule":     `Illuminate\Support\Facades\Schedule`,
	"Schema":       `Illuminate\Support\Facades\Schema`,
	"Session":      `Illuminate\Support\Facades\Session`,
	"Storage":      `Illuminate\Support\Facades\Storage`,
	"Str":          `Illuminate\Support\Str`,
	"Uri":          `Illuminate\Support\Uri`,
	"URL":          `Illuminate\Support\Facades\URL`,
	"Validator":    `Illuminate\Support\Facades\Validator`,
	"View":         `Illuminate\Support\Facades\View`,
	"Vite":         `Illuminate\Support\Facades\Vite`,
}

// Aliases are the class aliases Laravel's AliasLoader makes resolvable in the
// global namespace, so `Route::has()` works in a Blade view or an
// un-namespaced file without an import. They come from Laravel's defaults,
// the `aliases` of config/app.php and the extra.laravel.aliases of installed
// packages.
//
// Aliases is a symbols.ClassGenerator: a global class name that misses in the
// index and is an alias becomes a class extending its target, so the alias
// resolves, hovers and goes to its target. An alias whose target isn't
// indexed stays unknown.
type Aliases struct {
	rootPath string
	// byName maps a lower-case alias to its name as written and its target.
	byName atomic.Pointer[map[string]alias]
}

type alias struct {
	name, target string
}

// NewAliases returns the aliases of the Laravel app at rootPath. They are
// empty until Load reads them.
func NewAliases(rootPath string) *Aliases {
	return &Aliases{rootPath: rootPath}
}

// Load reads the aliases from the project: Laravel's defaults, then
// config/app.php, then installed packages, each overriding the one before as
// Laravel merges them.
func (a *Aliases) Load() {
	byName := make(map[string]alias, len(defaultAliases))
	add := func(name, target string) {
		name = strings.TrimSpace(name)
		target = strings.TrimPrefix(strings.TrimSpace(target), `\`)
		if name == "" || target == "" || strings.Contains(name, `\`) {
			return
		}
		byName[strings.ToLower(name)] = alias{name: name, target: target}
	}
	for name, target := range defaultAliases {
		add(name, target)
	}
	if data, err := os.ReadFile(filepath.Join(a.rootPath, "config", "app.php")); err == nil {
		for name, target := range configAliases(string(data)) {
			add(name, target)
		}
	}
	for name, target := range composer.LaravelPackageAliases(a.rootPath) {
		add(name, target)
	}
	a.byName.Store(&byName)
}

// Target returns the class an alias stands for.
func (a *Aliases) Target(name string) (string, bool) {
	m := a.byName.Load()
	if m == nil {
		return "", false
	}
	al, ok := (*m)[strings.ToLower(strings.TrimPrefix(name, `\`))]
	return al.target, ok
}

// Candidate implements symbols.ClassGenerator.
func (a *Aliases) Candidate(fqn string) bool {
	if strings.Contains(fqn, `\`) {
		return false
	}
	_, ok := a.Target(fqn)
	return ok
}

// Generate implements symbols.ClassGenerator.
func (a *Aliases) Generate(fqn string, idx *symbols.Index) (string, string, bool) {
	m := a.byName.Load()
	if m == nil {
		return "", "", false
	}
	al, ok := (*m)[strings.ToLower(fqn)]
	if !ok || strings.EqualFold(al.target, fqn) {
		return "", "", false
	}
	target := idx.LookupFold(al.target, func(sym *symbols.Symbol) bool {
		return sym.Kind == symbols.KindClass || sym.Kind == symbols.KindInterface
	})
	if target == nil {
		return "", "", false
	}
	kind := "class"
	if target.Kind == symbols.KindInterface {
		kind = "interface"
	}
	var b strings.Builder
	b.WriteString("<?php\n\n/**\n * Alias of @see \\")
	b.WriteString(target.FQN)
	b.WriteString(", registered by Laravel's AliasLoader.\n */\n")
	b.WriteString(kind)
	b.WriteString(" ")
	b.WriteString(al.name)
	b.WriteString(" extends \\")
	b.WriteString(target.FQN)
	b.WriteString(" {}\n")
	return b.String(), target.FQN, true
}

// configAliases reads the `aliases` entry of config/app.php: every
// `'Name' => Some\Class::class` or `'Name' => 'Some\Class'` pair in its value,
// including inside `Facade::defaultAliases()->merge([...])`.
func configAliases(source string) map[string]string {
	toks := phpTokens(source)
	file := parser.ParseFile(source)
	aliases := map[string]string{}
	for i := 0; i+1 < len(toks); i++ {
		if key, ok := firstStringLiteral(toks[i:]); !ok || key != "aliases" || toks[i+1].Kind != token.DoubleArrow {
			continue
		}
		end := valueEnd(toks, i+2)
		for j := i + 2; j+1 < end; j++ {
			name, ok := firstStringLiteral(toks[j:])
			if !ok || toks[j+1].Kind != token.DoubleArrow {
				continue
			}
			valueStop := min(valueEnd(toks, j+2), end)
			value := toks[j+2 : valueStop]
			if target, ok := parseClassConstExpr(value, file); ok {
				aliases[name] = target
			} else if target, ok := firstStringLiteral(value); ok && len(value) == 1 {
				aliases[name] = target
			}
			j = valueStop
		}
		return aliases
	}
	return aliases
}

func valueEnd(toks []phpToken, start int) int {
	depth := 0
	for i := start; i < len(toks); i++ {
		switch toks[i].Kind {
		case token.LeftParen, token.LeftBracket, token.Attribute, token.LeftBrace:
			depth++
		case token.RightParen, token.RightBracket, token.RightBrace:
			if depth == 0 {
				return i
			}
			depth--
		case token.Comma, token.Semicolon:
			if depth == 0 {
				return i
			}
		}
	}
	return len(toks)
}
