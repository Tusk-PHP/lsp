package symbols

import "testing"

func TestEnumsImplementTheirImplicitInterfaces(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///enums.php", `<?php
namespace App;

interface HasLabel {}

enum Suit { case Hearts; }

enum Status: string implements HasLabel { case Paid = 'paid'; }

enum Declared: int implements \UnitEnum { case One = 1; }
`)
	tests := []struct {
		enum, iface string
		want        bool
	}{
		{`App\Suit`, "UnitEnum", true},
		{`App\Suit`, "BackedEnum", false},
		{`App\Status`, "UnitEnum", true},
		{`App\Status`, "BackedEnum", true},
		{`App\Status`, `App\HasLabel`, true},
		{`App\Declared`, "BackedEnum", true},
	}
	for _, tt := range tests {
		if got := idx.IsSubtypeOf(tt.enum, tt.iface); got != tt.want {
			t.Errorf("IsSubtypeOf(%s, %s) = %v, want %v", tt.enum, tt.iface, got, tt.want)
		}
	}
	if got := idx.Lookup(`App\Declared`).Implements; len(got) != 2 {
		t.Errorf("Declared implements %q, want UnitEnum once and BackedEnum", got)
	}
}
