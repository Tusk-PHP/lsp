package diagnostics

import "testing"

func TestToolEnabled(t *testing.T) {
	on, off := true, false
	for _, tc := range []struct {
		name            string
		startsProcesses bool
		configured      *bool
		want            bool
	}{
		{"unset, where processes start", true, nil, true},
		{"turned on, where processes start", true, &on, true},
		{"turned off, where processes start", true, &off, false},
		{"unset, where processes can't start", false, nil, false},
		{"turned on, where processes can't start", false, &on, false},
	} {
		if got := toolEnabled(tc.startsProcesses, tc.configured); got != tc.want {
			t.Errorf("%s: toolEnabled = %v, want %v", tc.name, got, tc.want)
		}
	}
}
