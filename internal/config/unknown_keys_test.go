package config

import (
	"reflect"
	"slices"
	"testing"
)

type unknownKeysInner struct {
	Depth int `json:"depth"`
}

type unknownKeysProbe struct {
	Plain  string
	Hidden string            `json:"-"`
	Inner  *unknownKeysInner `json:"inner"`
	secret string
}

func unknownKeyMessages(data string) []string {
	var got []string
	for _, problem := range unknownKeys([]byte(data), reflect.TypeFor[unknownKeysProbe](), "") {
		got = append(got, problem.Error())
	}
	return got
}

func TestUnknownKeysFollowsEncodingJSONFieldNames(t *testing.T) {
	got := unknownKeyMessages(`{"plain": "a", "Hidden": "b", "secret": "c", "inner": {"depth": 1, "width": 2}}`)
	want := []string{
		`unknown setting "Hidden" is ignored`,
		`unknown setting "inner.width" is ignored`,
		`unknown setting "secret" is ignored`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("problems:\n got %q\nwant %q", got, want)
	}
}

func TestUnknownKeysIgnoresValuesThatAreNotObjects(t *testing.T) {
	for _, data := range []string{`[1, 2]`, `"text"`, `null`, `{"inner": null}`} {
		if got := unknownKeyMessages(data); len(got) != 0 {
			t.Errorf("%s: got %q, want nothing reported", data, got)
		}
	}
}
