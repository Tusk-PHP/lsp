package resolve

import "testing"

func TestUndocumentedSubclassPassesItsArgumentsOn(t *testing.T) {
	r, file, _ := setupBoxResolver(t, `<?php
namespace App;

class Person {}

/**
 * @template TKey
 * @template TValue
 */
class BaseCollection
{
    /** @return TValue */
    public function first() {}
}

/**
 * @template TKey
 * @template TModel
 * @extends BaseCollection<TKey, TModel>
 */
class ModelCollection extends BaseCollection {}

class PersonCollection extends ModelCollection {}

class Factory
{
    /** @return PersonCollection<int, Person> */
    public function people() {}
}
`)
	if got := r.ExpressionType("(new Factory())->people()->first()", ExprContext{File: file}).FullString(); got != "App\\Person" {
		t.Errorf("an undocumented subclass of a generic class passes its arguments to its parent: got %q", got)
	}
}
