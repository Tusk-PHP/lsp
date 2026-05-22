package analyzer

import "testing"

func TestCodeActionKindAllowed(t *testing.T) {
	tests := []struct {
		name string
		only []string
		kind string
		want bool
	}{
		{name: "empty only allows all", kind: "quickfix", want: true},
		{name: "exact match", only: []string{"quickfix"}, kind: "quickfix", want: true},
		{name: "parent kind matches child", only: []string{"source"}, kind: "source.organizeImports", want: true},
		{name: "nested parent matches child", only: []string{"refactor"}, kind: "refactor.extract", want: true},
		{name: "child does not match parent", only: []string{"refactor.extract"}, kind: "refactor", want: false},
		{name: "different branch rejected", only: []string{"source"}, kind: "quickfix", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codeActionKindAllowed(tt.only, tt.kind); got != tt.want {
				t.Fatalf("codeActionKindAllowed(%v, %q) = %v, want %v", tt.only, tt.kind, got, tt.want)
			}
		})
	}
}
