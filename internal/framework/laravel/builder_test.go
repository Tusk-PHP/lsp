package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestBuilderSecondNameFollowsLaravelsSignatures(t *testing.T) {
	tests := []struct {
		method    string
		argCount  int
		want      int
		wantFound bool
	}{
		{"withSum", 2, 1, true},
		{"pluck", 2, 1, true},
		{"pluck", 1, 1, true},
		{"whereColumn", 2, 1, true},
		{"whereColumn", 3, 2, true},
		{"orWhereColumn", 3, 2, true},
		{"orWhereColumn", 4, 2, true},
		{"where", 3, 0, false},
	}
	for _, tt := range tests {
		m, ok := BuilderMethodNamed(tt.method)
		if !ok {
			t.Fatalf("%s is not a builder method", tt.method)
		}
		if got, found := m.SecondNameAt(tt.argCount); got != tt.want || found != tt.wantFound {
			t.Errorf("%s with %d arguments: second name at %d, %v; want %d, %v", tt.method, tt.argCount, got, found, tt.want, tt.wantFound)
		}
	}
}

func TestBuilderArraysHoldNamesAsValuesOrKeys(t *testing.T) {
	for method, want := range map[string][2]bool{
		"select":  {true, false},
		"where":   {false, true},
		"with":    {true, true},
		"whereIn": {false, false},
	} {
		m, _ := BuilderMethodNamed(method)
		if got := [2]bool{m.NamesValues(), m.NamesKeys()}; got != want {
			t.Errorf("%s: names as values, keys = %v, want %v", method, got, want)
		}
	}
	if got := WithoutAlias("users.name as n"); got != "users.name" {
		t.Errorf("WithoutAlias = %q", got)
	}
}

func TestModelPropertyKinds(t *testing.T) {
	relation := &symbols.Symbol{Kind: symbols.KindProperty, IsVirtual: true, Name: "$author", FQN: `App\Post::$author`, Type: `?App\User`, DocComment: "BelongsTo" + RelationDocSuffix}
	many := &symbols.Symbol{Kind: symbols.KindProperty, IsVirtual: true, Name: "$comments", FQN: `App\Post::$comments`, Type: `Illuminate\Support\Collection`, DocComment: "HasMany" + RelationDocSuffix}
	accessor := &symbols.Symbol{Kind: symbols.KindProperty, IsVirtual: true, Name: "$full", DocComment: AccessorDocPrefix + "getFullAttribute"}
	column := &symbols.Symbol{Kind: symbols.KindProperty, IsVirtual: true, Name: "$title", FQN: `App\Post::$title`, DocComment: DatabaseColumnDoc}
	declared := &symbols.Symbol{Kind: symbols.KindProperty, Name: "$table"}
	method := &symbols.Symbol{Kind: symbols.KindMethod, Name: "save"}
	for _, c := range []struct {
		m                         *symbols.Symbol
		relation, col, db, liveDB bool
	}{
		{relation, true, false, false, false},
		{accessor, false, false, false, false},
		{column, false, true, true, true},
		{declared, false, true, false, false},
		{method, false, false, false, false},
	} {
		got := [4]bool{IsRelationProperty(c.m), IsColumnProperty(c.m), IsDBColumnProperty(c.m), IsLiveDBColumnProperty(c.m)}
		if want := [4]bool{c.relation, c.col, c.db, c.liveDB}; got != want {
			t.Errorf("%s: relation, column, db column, live db column = %v, want %v", c.m.Name, got, want)
		}
	}
	idx := symbols.NewIndex()
	idx.IndexFile("file:///post.php", "<?php\nnamespace App;\nclass Post {}\n")
	for _, m := range []*symbols.Symbol{relation, many, column} {
		idx.AddVirtualMember(`App\Post`, m)
	}
	for relationName, want := range map[string]string{"author": `App\User`, "comments": "", "title": "", "missing": ""} {
		if got := RelatedModel(idx, `App\Post`, relationName); got != want {
			t.Errorf("RelatedModel(%s) = %q, want %q", relationName, got, want)
		}
	}
}

func TestRelationTypes(t *testing.T) {
	for short, want := range map[string][3]bool{
		"BelongsTo":     {true, true, false},
		"BelongsToMany": {true, false, true},
		"Builder":       {false, false, false},
	} {
		if got := [3]bool{IsRelationType(short), IsSingularRelationType(short), IsPluralRelationType(short)}; got != want {
			t.Errorf("%s: relation, singular, plural = %v, want %v", short, got, want)
		}
	}
}
