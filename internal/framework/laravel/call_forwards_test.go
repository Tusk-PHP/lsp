package laravel

import "testing"

// An Eloquent model forwards what it doesn't have to a query builder for
// itself, and that builder finds the model's scopes by prefix or #[Scope].
func TestEloquentForwardsAndScopes(t *testing.T) {
	forwards := CallForwards()
	if got := forwards["Illuminate\\Database\\Eloquent\\Model"]; len(got) != 1 || got[0] != "\\Illuminate\\Database\\Eloquent\\Builder<static>" {
		t.Errorf("Model forwards to %v", got)
	}
	conv, ok := ScopeConventions()["Illuminate\\Database\\Eloquent\\Builder"]
	if !ok || conv.Prefix != "scope" || conv.Attribute != "Illuminate\\Database\\Eloquent\\Attributes\\Scope" {
		t.Errorf("Builder's scope convention = %+v, %v", conv, ok)
	}
}
