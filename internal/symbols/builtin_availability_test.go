package symbols

import "testing"

// TestLookupAvailabilityHandPicked verifies that a symbol present in the
// hand-authored builtinAvailabilityByName map is returned correctly.
func TestLookupAvailabilityHandPicked(t *testing.T) {
	got, ok := LookupAvailability("str_contains")
	if !ok {
		t.Fatal("LookupAvailability(\"str_contains\") returned ok=false, want true")
	}
	want := BuiltinAvailability{Since: "8.0"}
	if got != want {
		t.Errorf("LookupAvailability(\"str_contains\") = %+v, want %+v", got, want)
	}
}

// TestLookupAvailabilityFromGenerated verifies that a symbol present only in
// the generated table (not in the hand-authored map) is resolved.
func TestLookupAvailabilityFromGenerated(t *testing.T) {
	// array_is_list is in generatedBuiltinAvailability but not in
	// builtinAvailabilityByName.
	got, ok := LookupAvailability("array_is_list")
	if !ok {
		t.Fatal("LookupAvailability(\"array_is_list\") returned ok=false, want true")
	}
	want := BuiltinAvailability{Since: "8.1"}
	if got != want {
		t.Errorf("LookupAvailability(\"array_is_list\") = %+v, want %+v", got, want)
	}
}

// TestLookupAvailabilityUnknown verifies that a name not present in either map
// returns the zero value and ok=false.
func TestLookupAvailabilityUnknown(t *testing.T) {
	got, ok := LookupAvailability("definitely_not_a_php_function_xyz")
	if ok {
		t.Fatalf("LookupAvailability(unknown) returned ok=true, want false")
	}
	zero := BuiltinAvailability{}
	if got != zero {
		t.Errorf("LookupAvailability(unknown) = %+v, want zero value", got)
	}
}

// TestLookupAvailabilityHandOverridesGenerated verifies the precedence contract:
// hand-authored entries in builtinAvailabilityByName shadow any conflicting
// entry in generatedBuiltinAvailability. "str_contains" appears in both maps
// with identical values ({Since: "8.0"}), which is intentional — the test
// documents that the hand-authored map is consulted first (ordering in
// LookupAvailability), and equal values here are acceptable.
func TestLookupAvailabilityHandOverridesGenerated(t *testing.T) {
	// Confirm the hand-authored entry exists.
	handAuthored, inHand := builtinAvailabilityByName["str_contains"]
	if !inHand {
		t.Fatal("expected \"str_contains\" in builtinAvailabilityByName")
	}
	// Confirm the generated entry also exists.
	generated, inGenerated := generatedBuiltinAvailability["str_contains"]
	if !inGenerated {
		t.Fatal("expected \"str_contains\" in generatedBuiltinAvailability")
	}

	got, ok := LookupAvailability("str_contains")
	if !ok {
		t.Fatal("LookupAvailability(\"str_contains\") returned ok=false, want true")
	}
	// The hand-authored value must win.
	if got != handAuthored {
		t.Errorf("LookupAvailability(\"str_contains\") = %+v, want hand-authored %+v (generated was %+v)",
			got, handAuthored, generated)
	}
}
