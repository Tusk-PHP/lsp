package symbols

import "testing"

func TestLookupAvailabilityOfGeneratedConstants(t *testing.T) {
	if a, ok := LookupAvailability("DATE_ISO8601_EXPANDED"); !ok || a != (BuiltinAvailability{Since: "8.2"}) {
		t.Errorf("a constant that only a newer PHP declares has its version: %+v %v", a, ok)
	}
	for _, name := range []string{"ABDAY_1", "CURLOPT_ALTSVC_CTRL", "NOT_A_PHP_CONSTANT"} {
		if a, ok := LookupAvailability(name); ok {
			t.Errorf("%s has no version constraint to report: %+v", name, a)
		}
	}
}
