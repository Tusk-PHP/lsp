package laravel

import "testing"

func TestConventionMethod(t *testing.T) {
	cases := []struct {
		class, method string
		want          bool
	}{
		{`Illuminate\Notifications\Notification`, "toMail", true},
		{`\Illuminate\Notifications\Notification`, "VIA", true},
		{`Illuminate\Notifications\Notification`, "missing", false},
		{`App\Notifications\Invoice`, "toMail", false},
	}
	for _, c := range cases {
		if got := ConventionMethod(c.class, c.method); got != c.want {
			t.Errorf("ConventionMethod(%q, %q) = %v, want %v", c.class, c.method, got, c.want)
		}
	}
}
