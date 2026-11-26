package laravel

import "testing"

func TestViewCallIsNotTheEndOfANonASCIIName(t *testing.T) {
	if _, ok := ExtractViewCompletionContext("préview('ho"); ok {
		t.Error("préview() is not view()")
	}
	if ctx, ok := ExtractViewCompletionContext("$größe = view('ho"); !ok || ctx.Partial != "ho" {
		t.Errorf("view() after a non-ASCII variable is still a view call, got %+v, %v", ctx, ok)
	}
}
