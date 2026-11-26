package laravel

import "testing"

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
