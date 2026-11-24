package symbols

import "testing"

func memberNames(idx *Index, fqn string) map[string]int {
	out := map[string]int{}
	for _, m := range idx.GetClassMembers(fqn) {
		out[m.Name]++
	}
	return out
}

// TestIDEHelperMembersSurviveReindexingTheModel re-indexes a model file and
// then its IDE helper file, and checks the helper's @property members stay
// on the model exactly once.
func TestIDEHelperMembersSurviveReindexingTheModel(t *testing.T) {
	idx := NewIndex()
	const modelURI, helperURI = "file:///app/Models/Post.php", "file:///_ide_helper_models.php"
	model := `<?php
namespace App\Models;
class Post { public function publish() {} }
`
	helper := `<?php
namespace App\Models {
    /**
     * @property int $id
     * @property string $title
     */
    class Post {}
}
`
	idx.IndexFile(modelURI, model)
	idx.IndexIDEHelperFile(helperURI, helper)
	check := func(when string) {
		t.Helper()
		got := memberNames(idx, `App\Models\Post`)
		for _, name := range []string{"$id", "$title", "publish"} {
			if got[name] != 1 {
				t.Errorf("%s: %s appears %d times", when, name, got[name])
			}
		}
	}
	check("after indexing")

	idx.IndexFile(modelURI, model)
	check("after re-indexing the model")

	idx.IndexIDEHelperFile(helperURI, helper)
	check("after re-indexing the helper")

	// A property the model now declares itself wins over the helper's.
	idx.IndexFile(modelURI, `<?php
namespace App\Models;
class Post { public string $title; public function publish() {} }
`)
	check("after declaring $title")
	for _, m := range idx.GetClassMembers(`App\Models\Post`) {
		if m.Name == "$title" && m.URI != modelURI {
			t.Errorf("$title comes from %s, want the model", m.URI)
		}
	}

	// A helper that no longer lists a property takes it away.
	idx.IndexIDEHelperFile(helperURI, `<?php
namespace App\Models {
    /** @property string $title */
    class Post {}
}
`)
	if got := memberNames(idx, `App\Models\Post`); got["$id"] != 0 {
		t.Errorf("after dropping $id from the helper: it appears %d times", got["$id"])
	}
}

func TestReindexingUnchangedTextKeepsMembersAnalysisAdded(t *testing.T) {
	idx := NewIndex()
	const uri = "file:///app/Models/Post.php"
	source := "<?php\nnamespace App\\Models;\nclass Post { public function comments() {} }\n"
	addRelation := func() {
		idx.AddVirtualMember(`App\Models\Post`, &Symbol{Name: "$comments", FQN: `App\Models\Post::$comments`, Kind: KindProperty, URI: uri, IsVirtual: true})
	}
	idx.IndexFileWithSource(uri, source, SourceProject)
	addRelation()
	generation := idx.Generation()

	idx.IndexFileWithSource(uri, source, SourceProject)
	if idx.Lookup(`App\Models\Post::$comments`) == nil {
		t.Error("re-indexing the same text keeps the relation analysis added")
	}
	if idx.Generation() != generation {
		t.Error("re-indexing the same text changes nothing")
	}

	idx.IndexFileWithSource(uri, source, SourceVendor)
	if sym := idx.Lookup(`App\Models\Post`); sym == nil || sym.Source != SourceVendor {
		t.Errorf("the same text indexed as another kind of code is indexed again: %+v", sym)
	}

	idx.IndexFileWithSource(uri, source, SourceProject)
	addRelation()
	idx.IndexFileWithSource(uri, source+"\n", SourceProject)
	if idx.Lookup(`App\Models\Post::$comments`) != nil {
		t.Error("changed text is indexed again, dropping what analysis added for analysis to add again")
	}
}

func TestReindexingAnEmptyFileTracksIt(t *testing.T) {
	idx := NewIndex()
	idx.IndexFileWithSource("file:///empty.php", "", SourceProject)
	found := false
	for _, uri := range idx.GetAllFileURIs() {
		found = found || uri == "file:///empty.php"
	}
	if !found {
		t.Error("an empty file is indexed like any other")
	}
}
