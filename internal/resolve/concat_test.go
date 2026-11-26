package resolve

import "testing"

func TestConcatenationIsAString(t *testing.T) {
	r := NewResolver(nil)
	if got := r.exprType(`$dir . '/' . $name`, ExprContext{}).FullString(); got != "string" {
		t.Errorf("ExpressionType = %q, want string", got)
	}
}
