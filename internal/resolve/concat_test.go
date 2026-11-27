package resolve

import "testing"

func TestConcatenationIsAString(t *testing.T) {
	r := NewResolver(nil)
	if got := r.ExpressionType(`$dir . '/' . $name`, ExprContext{}).FullString(); got != "string" {
		t.Errorf("ExpressionType = %q, want string", got)
	}
}
