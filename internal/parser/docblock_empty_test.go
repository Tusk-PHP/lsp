package parser

import "testing"

func TestParseDocBlockOfAnEmptyDocBlockIsEmpty(t *testing.T) {
	for _, raw := range []string{"", "   ", "/**\n */", "/**\n *\n */", "/** */"} {
		if doc := ParseDocBlock(raw); doc == nil || !doc.IsEmpty() || doc.Deprecated || len(doc.Params) != 0 {
			t.Errorf("ParseDocBlock(%q) = %+v, want an empty doc block", raw, doc)
		}
	}
	for _, raw := range []string{"/** Summary. */", "/**\n * @internal\n */"} {
		if ParseDocBlock(raw).IsEmpty() {
			t.Errorf("ParseDocBlock(%q) is empty, want its summary or tag", raw)
		}
	}
}
