package hover

import (
	"strings"
	"testing"
)

func TestValuePreviewCutsAtATokenBoundary(t *testing.T) {
	long := strings.Repeat("self::FLAG|", 20) + "self::LAST"
	for _, tc := range []struct{ value, want string }{
		{"[1,2]", "[1,2]"},
		{long, strings.Repeat("self::FLAG|", 14) + "self::…"},
		{"'" + strings.Repeat("x", 200) + "'", "…"},
		{"f(" + strings.Repeat("\"{$a['k']}\",", 20) + ")", "f(" + strings.Repeat("\"{$a['k']}\",", 13) + "…)"},
		{"[[" + strings.Repeat("1,", 100) + "]]", "[[" + strings.Repeat("1,", 79) + "…]]"},
	} {
		if got := valuePreview(tc.value); got != tc.want {
			t.Errorf("valuePreview(%q)\n got %q\nwant %q", tc.value, got, tc.want)
		}
	}
}
