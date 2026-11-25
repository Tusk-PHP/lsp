package strict

import "testing"

func TestEnabled(t *testing.T) {
	for value, want := range map[string]bool{"": false, "0": false, "false": false, "1": true, "true": true, "YES": true, "on": true} {
		t.Setenv(EnvVar, value)
		if got := Enabled(); got != want {
			t.Errorf("%s=%q: got %v, want %v", EnvVar, value, got, want)
		}
	}
}
