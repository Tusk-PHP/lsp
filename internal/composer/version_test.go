package composer

import "testing"

func TestMinPHPFromConstraint(t *testing.T) {
	tests := []struct {
		constraint string
		want       string
	}{
		{"^8.1", "8.1"},
		{">=7.4", "7.4"},
		{"^8.2 || ^7.4", "7.4"},
		{"dev-main", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := MinPHPFromConstraint(tt.constraint); got != tt.want {
			t.Errorf("MinPHPFromConstraint(%q) = %q, want %q", tt.constraint, got, tt.want)
		}
	}
}

func TestCompareConstraint(t *testing.T) {
	tests := []struct {
		project    string
		constraint string
		want       int
	}{
		{"8.2", "^8.1", +1},
		{"8.1", "^8.1", +1},
		{"7.4", "^8.1", -1},
		{"8.0", "^7.4", +1},
		{"", "^8.1", 0},
		{"8.1", "dev-main", 0},
		{"8.1", "", 0},
	}
	for _, tt := range tests {
		if got := CompareConstraint(tt.project, tt.constraint); got != tt.want {
			t.Errorf("CompareConstraint(%q, %q) = %d, want %d", tt.project, tt.constraint, got, tt.want)
		}
	}
}

func TestNormalizeMajorMinor(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"8.2.1", "8.2"},
		{"v10.4.1", "10.4"},
		{"8", "8.0"},
		{"abc", ""},
	}
	for _, tt := range tests {
		if got := normalizeMajorMinor(tt.in); got != tt.want {
			t.Errorf("normalizeMajorMinor(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
