package models

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

// parseMigrationFile applies one migration file's schema changes to
// tableColumns, read from its tokens. Only what the migration does when it
// runs counts: the body of up() (or, in a migration without one, everything
// outside down()), so a rollback's dropColumn() doesn't remove a column.
func parseMigrationFile(source string, tableColumns map[string]map[string]*MigrationColumn) {
	toks := migrationTokens{source, lexer.WholeLiterals(lexer.Significant(lexer.Lex(source, token.Latest)))}
	ranges := methodBodies(toks, "up")
	if len(ranges) == 0 {
		ranges = outside(methodBodies(toks, "down"), len(toks.all))
	}
	for _, r := range ranges {
		scanSchemaCalls(toks, r[0], r[1], tableColumns)
	}
}

type migrationTokens struct {
	src string
	all []lexer.Token
}

func (t migrationTokens) kind(i int) token.Kind {
	return t.all[i].Kind
}

func (t migrationTokens) text(i int) string {
	return t.all[i].Text(t.src)
}

// methodBodies returns the token ranges, inside the braces, of every
// `function name(...) {...}`.
func methodBodies(toks migrationTokens, name string) [][2]int {
	var out [][2]int
	for i := 0; i+2 < len(toks.all); i++ {
		if toks.kind(i) != token.Function || !strings.EqualFold(toks.text(i+1), name) || toks.kind(i+2) != token.LeftParen {
			continue
		}
		closeParams := lexer.MatchingClose(toks.all, i+2)
		if closeParams < 0 {
			continue
		}
		open := -1
		for j := closeParams + 1; j < len(toks.all); j++ {
			if toks.kind(j) == token.LeftBrace {
				open = j
				break
			}
			if toks.kind(j) == token.Semicolon {
				break
			}
		}
		if open < 0 {
			continue
		}
		if end := lexer.MatchingClose(toks.all, open); end > open {
			out = append(out, [2]int{open + 1, end})
			i = end
		}
	}
	return out
}

// outside returns the ranges of [0, n) not covered by skip, which is sorted.
func outside(skip [][2]int, n int) [][2]int {
	var out [][2]int
	at := 0
	for _, r := range skip {
		if r[0] > at {
			out = append(out, [2]int{at, r[0]})
		}
		at = r[1]
	}
	if at < n {
		out = append(out, [2]int{at, n})
	}
	return out
}

// callArgs splits the arguments of the call whose `(` is at open into token
// ranges, and returns the index of its `)`.
func callArgs(toks migrationTokens, open int) ([][2]int, int) {
	end := lexer.MatchingClose(toks.all, open)
	if end < 0 {
		return nil, -1
	}
	var args [][2]int
	start := open + 1
	depth := 0
	for i := open + 1; i < end; i++ {
		switch toks.kind(i) {
		case token.LeftParen, token.LeftBracket, token.LeftBrace:
			depth++
		case token.RightParen, token.RightBracket, token.RightBrace:
			depth--
		case token.Comma:
			if depth == 0 {
				args = append(args, [2]int{start, i})
				start = i + 1
			}
		}
	}
	if start < end {
		args = append(args, [2]int{start, end})
	}
	return args, end
}

// literal returns the value of a plain string literal, unquoted. A string
// with interpolation, or anything else, gives false.
func (t migrationTokens) literal(i int) (string, bool) {
	v := t.text(i)
	if t.kind(i) != token.ConstantString {
		return "", false
	}
	inner := v[1 : len(v)-1]
	if v[0] == '"' && strings.ContainsAny(inner, "$\\") {
		return "", false
	}
	if v[0] == '\'' && strings.Contains(inner, "\\") {
		return "", false
	}
	return inner, true
}

// argString is the string literal an argument consists of.
func argString(toks migrationTokens, arg [2]int) (string, bool) {
	if arg[1]-arg[0] != 1 {
		return "", false
	}
	return toks.literal(arg[0])
}

// argStrings reads an argument that is a string or an array of strings.
func argStrings(toks migrationTokens, arg [2]int) []string {
	if s, ok := argString(toks, arg); ok {
		return []string{s}
	}
	if arg[1]-arg[0] < 2 || toks.kind(arg[0]) != token.LeftBracket {
		return nil
	}
	var out []string
	for i := arg[0] + 1; i < arg[1]; i++ {
		if s, ok := toks.literal(i); ok {
			out = append(out, s)
		}
	}
	return out
}

// scanSchemaCalls applies the Schema::create(), table(), drop(),
// dropIfExists() and rename() calls in toks[lo:hi].
func scanSchemaCalls(toks migrationTokens, lo, hi int, tables map[string]map[string]*MigrationColumn) {
	for i := lo; i+3 < hi; i++ {
		if !isSchemaFacade(toks.text(i)) || toks.kind(i+1) != token.DoubleColon {
			continue
		}
		m := i + 2
		// Schema::connection('other')->create(...)
		if strings.EqualFold(toks.text(m), "connection") && m+1 < hi && toks.kind(m+1) == token.LeftParen {
			if _, end := callArgs(toks, m+1); end > 0 && end+2 < hi && toks.kind(end+1) == token.ObjectOperator {
				m = end + 2
			}
		}
		if m+1 >= hi || toks.kind(m+1) != token.LeftParen {
			continue
		}
		args, end := callArgs(toks, m+1)
		if end < 0 || len(args) == 0 {
			continue
		}
		table, ok := argString(toks, args[0])
		if !ok {
			i = end
			continue
		}
		switch strings.ToLower(toks.text(m)) {
		case "create":
			tables[table] = map[string]*MigrationColumn{}
			if len(args) > 1 {
				applyBlueprint(toks, args[1], tables[table])
			}
		case "table":
			if tables[table] == nil {
				tables[table] = map[string]*MigrationColumn{}
			}
			if len(args) > 1 {
				applyBlueprint(toks, args[1], tables[table])
			}
		case "drop", "dropifexists":
			delete(tables, table)
		case "rename":
			if len(args) > 1 {
				if to, ok := argString(toks, args[1]); ok {
					if cols, found := tables[table]; found {
						tables[to] = cols
						delete(tables, table)
					}
				}
			}
		}
		i = end
	}
}

func isSchemaFacade(name string) bool {
	return name[strings.LastIndexByte(name, '\\')+1:] == "Schema"
}

// applyBlueprint applies the statements of a `function (Blueprint $table) {}`
// closure argument to cols, following whatever the closure names its
// parameter.
func applyBlueprint(toks migrationTokens, arg [2]int, cols map[string]*MigrationColumn) {
	fn := arg[0]
	if toks.kind(fn) == token.Static {
		fn++
	}
	if fn >= arg[1] || toks.kind(fn) != token.Function || fn+1 >= arg[1] || toks.kind(fn+1) != token.LeftParen {
		return
	}
	closeParams := lexer.MatchingClose(toks.all, fn+1)
	if closeParams < 0 {
		return
	}
	varName := ""
	for j := fn + 2; j < closeParams; j++ {
		if toks.kind(j) == token.Variable {
			varName = toks.text(j)
			break
		}
	}
	open := -1
	for j := closeParams + 1; j < arg[1]; j++ {
		if toks.kind(j) == token.LeftBrace {
			open = j
			break
		}
	}
	if varName == "" || open < 0 {
		return
	}
	end := lexer.MatchingClose(toks.all, open)
	if end < 0 {
		return
	}
	for j := open + 1; j+3 < end; j++ {
		if toks.kind(j) != token.Variable || toks.text(j) != varName || toks.kind(j+1) != token.ObjectOperator || toks.kind(j+3) != token.LeftParen {
			continue
		}
		method := toks.text(j + 2)
		args, callEnd := callArgs(toks, j+3)
		if callEnd < 0 {
			continue
		}
		// The modifiers chained after the call, up to the end of the statement.
		var mods []blueprintModifier
		k := callEnd + 1
		for k+2 < end && toks.kind(k) == token.ObjectOperator && toks.kind(k+2) == token.LeftParen {
			modArgs, modEnd := callArgs(toks, k+2)
			if modEnd < 0 {
				break
			}
			mods = append(mods, blueprintModifier{name: toks.text(k + 1), args: modArgs})
			k = modEnd + 1
		}
		applyColumnCall(toks, method, args, mods, cols)
		j = k - 1
	}
}

type blueprintModifier struct {
	name string
	args [][2]int
}

// morphColumns are the Blueprint methods that add `{name}_type` and
// `{name}_id`, with the id's PHP type and whether both are nullable.
var morphColumns = map[string]struct {
	idType   string
	nullable bool
}{
	"morphs": {"int", false}, "nullableMorphs": {"int", true},
	"numericMorphs": {"int", false}, "nullableNumericMorphs": {"int", true},
	"uuidMorphs": {"string", false}, "nullableUuidMorphs": {"string", true},
	"ulidMorphs": {"string", false}, "nullableUlidMorphs": {"string", true},
}

var incrementsMethods = map[string]bool{
	"increments": true, "bigIncrements": true, "mediumIncrements": true,
	"smallIncrements": true, "tinyIncrements": true, "integerIncrements": true,
}

func applyColumnCall(toks migrationTokens, method string, args [][2]int, mods []blueprintModifier, cols map[string]*MigrationColumn) {
	first := ""
	if len(args) > 0 {
		first, _ = argString(toks, args[0])
	}
	add := func(name, phpType, rawType string, nullable bool) {
		if name == "" {
			return
		}
		col := &MigrationColumn{Name: name, Type: phpType, RawType: rawType, Nullable: nullable}
		for _, m := range mods {
			switch m.name {
			case "nullable":
				col.Nullable = len(m.args) == 0 || toks.text(m.args[0][0]) != "false"
			case "default":
				if len(m.args) > 0 {
					col.Default = joinTokens(toks, m.args[0])
				}
			}
		}
		cols[name] = col
	}
	drop := func(names ...string) {
		for _, n := range names {
			delete(cols, n)
		}
	}

	if phpType, ok := blueprintTypeMap[method]; ok {
		add(first, phpType, method, false)
		return
	}
	if incrementsMethods[method] {
		add(first, "int", method, false)
		return
	}
	if m, ok := morphColumns[method]; ok {
		if first != "" {
			add(first+"_type", "string", "string", m.nullable)
			add(first+"_id", m.idType, method, m.nullable)
		}
		return
	}
	switch method {
	case "id":
		if first == "" && len(args) == 0 {
			first = "id"
		}
		add(first, "int", "bigint", false)
	case "timestamps", "timestampsTz", "nullableTimestamps":
		add("created_at", "\\DateTimeInterface", "datetime", true)
		add("updated_at", "\\DateTimeInterface", "datetime", true)
	case "softDeletes", "softDeletesTz":
		if first == "" && len(args) == 0 {
			first = "deleted_at"
		}
		add(first, "\\DateTimeInterface", "datetime", true)
	case "rememberToken":
		add("remember_token", "string", "string", true)
	case "foreignIdFor":
		// foreignIdFor(User::class) is user_id; a second argument names it.
		name := ""
		if len(args) > 1 {
			name, _ = argString(toks, args[1])
		} else if len(args) == 1 && args[0][1]-args[0][0] >= 3 &&
			toks.kind(args[0][1]-1) == token.Class && toks.kind(args[0][1]-2) == token.DoubleColon {
			class := toks.text(args[0][1] - 3)
			name = toSnakeCase(class[strings.LastIndex(class, "\\")+1:]) + "_id"
		}
		add(name, "int", "foreignId", false)
	case "dropColumn", "dropColumns":
		for _, a := range args {
			drop(argStrings(toks, a)...)
		}
	case "renameColumn":
		if len(args) > 1 {
			if to, ok := argString(toks, args[1]); ok && first != "" {
				if col, found := cols[first]; found {
					delete(cols, first)
					col.Name = to
					cols[to] = col
				}
			}
		}
	case "dropTimestamps", "dropTimestampsTz":
		drop("created_at", "updated_at")
	case "dropSoftDeletes", "dropSoftDeletesTz":
		if first == "" {
			first = "deleted_at"
		}
		drop(first)
	case "dropRememberToken":
		drop("remember_token")
	case "dropMorphs":
		if first != "" {
			drop(first+"_type", first+"_id")
		}
	case "dropConstrainedForeignId":
		drop(first)
	}
}

func joinTokens(toks migrationTokens, r [2]int) string {
	var b strings.Builder
	for i := r[0]; i < r[1]; i++ {
		b.WriteString(toks.text(i))
	}
	return b.String()
}
