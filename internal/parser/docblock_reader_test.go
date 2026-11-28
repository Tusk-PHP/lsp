package parser

import (
	"reflect"
	"testing"
)

func TestDocBlockReadsTypesOverSeveralLines(t *testing.T) {
	doc := ParseDocBlock(`/**
 * Load a row.
 *
 * @param array{
 *     id: int,
 *     tags: list<array{
 *         name: string,
 *     }>,
 * } $row The row to load
 * @param callable(
 *     int,
 *     string
 * ): void $done
 * @var array<
 *     string,
 *     int
 * > $counts
 * @return array{
 *     id: int,
 *     name: string,
 * } The loaded row
 */`)
	if doc == nil {
		t.Fatal("nil doc")
	}
	if doc.Summary != "Load a row." {
		t.Errorf("summary = %q", doc.Summary)
	}
	wantParams := []DocParam{
		{Type: "array{id: int, tags: list<array{name: string}>}", Name: "$row", Description: "The row to load"},
		{Type: "callable(int, string): void", Name: "$done"},
	}
	if len(doc.Params) != len(wantParams) {
		t.Fatalf("params = %+v", doc.Params)
	}
	for i, want := range wantParams {
		got := doc.Params[i]
		if got.Type != want.Type || got.Name != want.Name || got.Description != want.Description {
			t.Errorf("param %d = %q %q %q, want %q %q %q", i, got.Type, got.Name, got.Description, want.Type, want.Name, want.Description)
		}
		if got.ParsedType == nil {
			t.Errorf("param %d type %q did not parse", i, got.Type)
		}
	}
	if len(doc.Vars) != 1 || doc.Vars[0].Type != "array<string, int>" || doc.Vars[0].Name != "$counts" {
		t.Errorf("vars = %+v", doc.Vars)
	}
	if doc.Return.Type != "array{id: int, name: string}" || doc.Return.Description != "The loaded row" {
		t.Errorf("return = %q %q", doc.Return.Type, doc.Return.Description)
	}
	if doc.Return.ParsedType == nil || len(doc.Return.ParsedType.ShapeFields) != 2 {
		t.Errorf("return parsed = %+v", doc.Return.ParsedType)
	}
}

func TestDocBlockReadsClassTagsOverSeveralLines(t *testing.T) {
	doc := ParseDocBlock(`/**
 * @property array{
 *     theme: string,
 * } $settings The user's settings
 * @method static array{
 *     id: int,
 * } find(
 *     int $id,
 *     array{with: list<string>} $options = [],
 * ) Finds a row
 * @phpstan-type Row array{
 *     id: int,
 *     meta: array{
 *         'a{': int,
 *     },
 * }
 */`)
	if doc == nil {
		t.Fatal("nil doc")
	}
	if len(doc.Properties) != 1 {
		t.Fatalf("properties = %+v", doc.Properties)
	}
	if p := doc.Properties[0]; p.Type != "array{theme: string}" || p.Name != "settings" || p.Description != "The user's settings" {
		t.Errorf("property = %q %q %q", p.Type, p.Name, p.Description)
	}
	if len(doc.Methods) != 1 {
		t.Fatalf("methods = %+v", doc.Methods)
	}
	m := doc.Methods[0]
	if !m.IsStatic || m.ReturnType != "array{id: int}" || m.Name != "find" || m.Description != "Finds a row" {
		t.Errorf("method = static %v %q %q %q", m.IsStatic, m.ReturnType, m.Name, m.Description)
	}
	wantParams := []DocMethodParam{
		{Type: "int", Name: "$id"},
		{Type: "array{with: list<string>}", Name: "$options", Default: "[]"},
	}
	if !reflect.DeepEqual(m.Params, wantParams) {
		t.Errorf("method params = %+v, want %+v", m.Params, wantParams)
	}
	if len(doc.TypeAliases) != 1 {
		t.Fatalf("type aliases = %+v", doc.TypeAliases)
	}
	if a := doc.TypeAliases[0]; a.Name != "Row" || a.Type != "array{id: int, meta: array{'a{': int}}" || a.ParsedType == nil {
		t.Errorf("type alias = %q %q %v", a.Name, a.Type, a.ParsedType)
	}
}

func TestDocBlockUnbalancedTagStopsAtNextTag(t *testing.T) {
	doc := ParseDocBlock(`/**
 * @return array{
 *     id: int,
 * @param int $x
 * @param array<int $y
 */`)
	if doc == nil {
		t.Fatal("nil doc")
	}
	if got := doc.Tags["return"]; len(got) != 1 || got[0] != "array{id: int," {
		t.Errorf("return tag = %q", got)
	}
	if got := doc.Tags["param"]; len(got) != 2 || got[0] != "int $x" || got[1] != "array<int $y" {
		t.Errorf("param tags = %q", got)
	}
}

func TestDocBlockQuotedBracketsDoNotOpenATag(t *testing.T) {
	doc := ParseDocBlock(`/**
 * @return 'a{'|"b("|int
 * Text after the tag.
 */`)
	if doc == nil {
		t.Fatal("nil doc")
	}
	if doc.Return.Type != `'a{'|"b("|int` {
		t.Errorf("return = %q", doc.Return.Type)
	}
	if doc.Summary != "Text after the tag." {
		t.Errorf("summary = %q", doc.Summary)
	}
}

func TestDocTagOffsetsPointIntoTheComment(t *testing.T) {
	comment := "/**\n * @return array{\n *     id: Foo,\n *     name: Bar,\n * } rest\n * @param Baz $b\n */"
	var tags []DocTag
	for tag := range DocTags(comment) {
		tags = append(tags, tag)
	}
	if len(tags) != 2 {
		t.Fatalf("tags = %+v", tags)
	}
	for _, tag := range tags {
		for i := range len(tag.Value) {
			if c := tag.Value[i]; c != ' ' && comment[tag.Offset(i)] != c {
				t.Errorf("%s: Value[%d] = %q, comment at Offset = %q", tag.Name, i, c, comment[tag.Offset(i)])
			}
		}
	}
}

func TestDocTagsStopWhenTheReaderDoes(t *testing.T) {
	comment := "/**\n * @param Foo $a\n * @param Bar $b\n * @return Baz\n */"
	var names []string
	for tag := range DocTags(comment) {
		names = append(names, tag.Value)
		break
	}
	if len(names) != 1 || names[0] != "Foo $a" {
		t.Errorf("a reader that stops after the first tag got %q", names)
	}
}

func TestDocTagsReadAnAnnotationOpenedInItsName(t *testing.T) {
	comment := "/**\n * @Route(\n *     \"/path\",\n * )\n */"
	var tags []DocTag
	for tag := range DocTags(comment) {
		tags = append(tags, tag)
	}
	if len(tags) != 1 || tags[0].Name != "Route(" || tags[0].Value != "\"/path\")" {
		t.Fatalf("tags = %+v, want @Route( with the lines up to its closer", tags)
	}
	for i := range len(tags[0].Value) {
		if c := tags[0].Value[i]; comment[tags[0].Offset(i)] != c {
			t.Errorf("Value[%d] = %q, comment at Offset = %q", i, c, comment[tags[0].Offset(i)])
		}
	}
}

func TestDocTagsReadLinesIndentedWithNonASCIISpace(t *testing.T) {
	comment := "/**\n * @param Foo $a\n　*　@return Bar\n */"
	var values []string
	for tag := range DocTags(comment) {
		values = append(values, tag.Name+" "+tag.Value)
	}
	if want := []string{"param Foo $a", "return Bar"}; !reflect.DeepEqual(values, want) {
		t.Errorf("tags = %q, want %q", values, want)
	}
}
