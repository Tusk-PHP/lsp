package symbols

import "testing"

func TestCallForwardsAndScopeConventions(t *testing.T) {
	idx := NewIndex()
	if got := idx.CallForwards("Model"); got != nil {
		t.Errorf("a new index forwards Model to %v", got)
	}
	if _, ok := idx.ScopeConventionOf("Builder"); ok {
		t.Error("a new index has a scope convention for Builder")
	}

	idx.SetCallForwards(map[string][]string{"Model": {"\\Builder<static>"}})
	idx.SetScopeConventions(map[string]ScopeConvention{"Builder": {Prefix: "scope", Attribute: "Scope"}})

	if got := idx.CallForwards("Model"); len(got) != 1 || got[0] != "\\Builder<static>" {
		t.Errorf("CallForwards(Model) = %v", got)
	}
	if got := idx.CallForwards("Other"); got != nil {
		t.Errorf("CallForwards(Other) = %v, want nil", got)
	}
	if c, ok := idx.ScopeConventionOf("Builder"); !ok || c.Prefix != "scope" || c.Attribute != "Scope" {
		t.Errorf("ScopeConventionOf(Builder) = %+v, %v", c, ok)
	}
	if _, ok := idx.ScopeConventionOf("Other"); ok {
		t.Error("ScopeConventionOf(Other) found a convention")
	}
}
