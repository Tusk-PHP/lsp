package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func queriedModelIndex() *symbols.Index {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", "<?php\nnamespace Illuminate\\Database\\Eloquent;\nabstract class Model {}\nclass Builder {}\n")
	idx.IndexFile("file:///vendor/Relations.php", "<?php\nnamespace Illuminate\\Database\\Eloquent\\Relations;\nabstract class Relation {}\nclass HasMany extends Relation {}\nclass BelongsToMany extends Relation {}\nabstract class HasOneOrManyThrough extends Relation {}\nclass HasManyThrough extends HasOneOrManyThrough {}\n")
	idx.IndexFile("file:///app/Models.php", "<?php\nnamespace App\\Models;\nclass Post extends \\Illuminate\\Database\\Eloquent\\Model {}\nclass Video extends \\Illuminate\\Database\\Eloquent\\Model {}\nclass Tags extends \\Illuminate\\Database\\Eloquent\\Relations\\BelongsToMany {}\nclass Plain {}\n")
	return idx
}

func TestModelQueriedBy(t *testing.T) {
	idx := queriedModelIndex()
	post := resolve.ResolvedType{FQN: `App\Models\Post`}
	video := resolve.ResolvedType{FQN: `App\Models\Video`}
	of := func(fqn string, params ...resolve.ResolvedType) resolve.ResolvedType {
		return resolve.ResolvedType{FQN: fqn, Params: params}
	}
	union := resolve.ResolvedType{FQN: post.FQN, Union: []resolve.ResolvedType{post, video}}
	intersection := resolve.ResolvedType{FQN: post.FQN, Intersection: []resolve.ResolvedType{post, video}}
	relations := `Illuminate\Database\Eloquent\Relations\`
	cases := []struct {
		name string
		rt   resolve.ResolvedType
		want QueriedModel
	}{
		{"a model", post, QueriedModel{FQN: post.FQN}},
		{"a builder of a model", of(EloquentBuilder, post), QueriedModel{FQN: post.FQN}},
		{"a builder of no known model", of(EloquentBuilder), QueriedModel{}},
		{"a builder of something else", of(EloquentBuilder, of(`App\Models\Plain`)), QueriedModel{}},
		{"a class that isn't a builder", of(`App\Models\Plain`, post), QueriedModel{}},
		{"a relation", of(relations+"HasMany", post), QueriedModel{FQN: post.FQN}},
		{"a relation to a union", of(relations+"HasMany", union), QueriedModel{}},
		{"a relation to an intersection", of(relations+"HasMany", intersection), QueriedModel{}},
		{"a union of models", union, QueriedModel{}},
	}
	for _, c := range cases {
		if got := ModelQueriedBy(idx, c.rt); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestIsModel(t *testing.T) {
	idx := queriedModelIndex()
	if !IsModel(idx, `App\Models\Post`) || IsModel(idx, `App\Models\Plain`) || IsModel(idx, EloquentModel) {
		t.Error("a model is a class that extends Eloquent's Model")
	}
}
