package checks

import (
	"fmt"
	"strings"
	"testing"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/rule"
)

// mockMemberChecker implements MemberChecker for testing.
type mockMemberChecker struct {
	columns      map[string]map[string]bool // modelFQN → set of column names
	dbColumns    map[string]map[string]bool
	relations    map[string]map[string]bool
	relatedModel map[string]map[string]string // modelFQN → relationName → relatedModelFQN
	partial      map[string]bool              // models whose members aren't all known
}

func (m *mockMemberChecker) IsColumn(modelFQN, name string) bool {
	if cols, ok := m.columns[modelFQN]; ok {
		return cols[name]
	}
	return false
}

func (m *mockMemberChecker) IsDBColumn(modelFQN, name string) bool {
	if cols, ok := m.dbColumns[modelFQN]; ok {
		return cols[name]
	}
	return false
}

func (m *mockMemberChecker) IsRelation(modelFQN, name string) bool {
	if rels, ok := m.relations[modelFQN]; ok {
		return rels[name]
	}
	return false
}

func (m *mockMemberChecker) RelatedModelFQN(modelFQN, relationName string) string {
	if rels, ok := m.relatedModel[modelFQN]; ok {
		return rels[relationName]
	}
	return ""
}

// KnowsColumns and KnowsRelations treat a model listed in columns as fully
// known, unless it is listed in partial.
func (m *mockMemberChecker) KnowsColumns(modelFQN string) bool {
	_, ok := m.columns[modelFQN]
	return ok && !m.partial[modelFQN]
}

func (m *mockMemberChecker) KnowsRelations(modelFQN string) bool {
	return m.KnowsColumns(modelFQN)
}

func setupBuilderArgRule() *InvalidBuilderArgRule {
	members := &mockMemberChecker{
		columns: map[string]map[string]bool{
			"App\\Models\\Category": {"id": true, "name": true, "slug": true, "created_at": true},
			"App\\Models\\Product":  {"id": true, "category_id": true, "title": true, "price": true},
		},
		dbColumns: map[string]map[string]bool{
			"App\\Models\\Category": {"id": true, "name": true, "slug": true, "created_at": true},
			"App\\Models\\Product":  {"id": true, "category_id": true, "title": true, "price": true},
		},
		relations: map[string]map[string]bool{
			"App\\Models\\Category": {"products": true},
			"App\\Models\\Product":  {"category": true},
		},
		relatedModel: map[string]map[string]string{
			"App\\Models\\Category": {"products": "App\\Models\\Product"},
			"App\\Models\\Product":  {"category": "App\\Models\\Category"},
		},
	}

	resolver := func(file *parser.FileNode, receiver ast.Expr) frameworklaravel.QueriedModel {
		prefix := receiver.Bounds().Text(file.Source)
		if strings.Contains(prefix, "Category") {
			return frameworklaravel.QueriedModel{FQN: "App\\Models\\Category"}
		}
		if strings.Contains(prefix, "Product") {
			return frameworklaravel.QueriedModel{FQN: "App\\Models\\Product"}
		}
		return frameworklaravel.QueriedModel{}
	}

	return &InvalidBuilderArgRule{
		ModelResolver: resolver,
		Members:       members,
	}
}

func TestInvalidBuilderArgRule(t *testing.T) {
	checker := setupBuilderArgRule()

	t.Run("unknown column in where flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::where('nonexistent', 'value');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-column"})
		if !strings.Contains(findings[0].Message, "nonexistent") {
			t.Errorf("expected message to contain 'nonexistent', got: %s", findings[0].Message)
		}
	})

	t.Run("known column in where not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::where('name', 'value');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("unknown relation in with flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with('nonexistent');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-relation"})
	})

	t.Run("known relation in with not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with('products');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("unknown DB column in get flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::get(['nonexistent']);
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-column"})
	})

	t.Run("known columns in get not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::get(['id', 'name']);
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("array with one bad relation flags only bad one", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with(['products', 'bad']);
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-relation"})
		if !strings.Contains(findings[0].Message, "bad") {
			t.Errorf("expected message about 'bad', got: %s", findings[0].Message)
		}
	})

	t.Run("withCount with alias strips alias", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::withCount(['products as cnt']);
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("incomplete string no closing quote not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::where('na
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("non-model receiver not flagged", func(t *testing.T) {
		source := `<?php
$container->get('service');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("Product orderBy known column not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Product;
Product::orderBy('title');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("Product orderBy unknown name not flagged: it may be a select alias", func(t *testing.T) {
		source := `<?php
use App\Models\Product;
Product::selectRaw('count(*) as total')->orderBy('total');
Product::withCount('category')->having('category_count', '>', 1);
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("severity is warning", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::where('bad_col', 'x');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		if len(findings) != 1 {
			t.Fatalf("expected 1 finding, got %d", len(findings))
		}
		if findings[0].Severity != rule.SeverityWarning {
			t.Errorf("expected rule.SeverityWarning, got %d", findings[0].Severity)
		}
	})

	t.Run("dot notation validates first segment", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with('products.tags');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		// 'products' exists as relation, so first segment is valid
		assertNoFindings(t, findings)
	})

	t.Run("dot notation flags bad first segment", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with('nonexistent.tags');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-relation"})
	})

	t.Run("withSum second arg validates column on related model", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::withSum('products', 'price');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("withSum second arg flags unknown column on related model", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::withSum('products', 'nonexistent');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-column"})
		if !strings.Contains(findings[0].Message, "Product") {
			t.Errorf("expected message to reference related model 'Product', got: %s", findings[0].Message)
		}
	})

	t.Run("withAvg second arg validates column on related model", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::withAvg('products', 'price');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("orderBy with dot notation skipped", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::orderBy('relation.column');
`
		file := parser.ParseFile(source, token.Latest)
		findings := checker.Check(file, source, nil)
		// Dot-notation in column methods is join-qualified — skip validation
		assertNoFindings(t, findings)
	})
}

// TestInvalidBuilderArgRuleStaysQuietWhenUnsure covers code the rule can't
// judge: calls in comments and strings, arguments that are not plain names,
// and models whose columns or relations aren't all known.
func TestInvalidBuilderArgRuleStaysQuietWhenUnsure(t *testing.T) {
	checker := setupBuilderArgRule()
	checker.Members.(*mockMemberChecker).columns["App\\Models\\Draft"] = map[string]bool{"id": true}
	checker.Members.(*mockMemberChecker).partial = map[string]bool{"App\\Models\\Draft": true}
	checker.ModelResolver = func(file *parser.FileNode, receiver ast.Expr) frameworklaravel.QueriedModel {
		prefix := receiver.Bounds().Text(file.Source)
		switch {
		case strings.Contains(prefix, "Draft"):
			return frameworklaravel.QueriedModel{FQN: "App\\Models\\Draft"}
		case strings.Contains(prefix, "Category"):
			return frameworklaravel.QueriedModel{FQN: "App\\Models\\Category"}
		}
		return frameworklaravel.QueriedModel{}
	}
	source := `<?php
use App\Models\Category;
use App\Models\Draft;

/**
 * Not Category::where('reportable_type', ...) but the relation.
 */
// Category::with('commented_out');
$sql = "Category::where('in_a_string', 1)";
Category::where('options->locale', 'en');
Category::select('*');
Category::select('categories.*');
Category::with('products:id,title');
Draft::where('not_in_migrations', 1);
Draft::with('fromAnUnindexedTrait');
`
	file := parser.ParseFile(source, token.Latest)
	assertNoFindings(t, checker.Check(file, source, nil))
}

// TestInvalidBuilderArgRulePassesCallReceiver checks that the model resolver
// gets each builder call's receiver node, even when the call starts on an
// earlier line.
func TestInvalidBuilderArgRulePassesCallReceiver(t *testing.T) {
	checker := setupBuilderArgRule()
	var receivers []string
	checker.ModelResolver = func(file *parser.FileNode, receiver ast.Expr) frameworklaravel.QueriedModel {
		receivers = append(receivers, receiver.Bounds().Text(file.Source))
		return frameworklaravel.QueriedModel{}
	}
	source := `<?php
Category::query()->where('name', 1)->with('products');
Category::query()
    ->with('products');
`
	file := parser.ParseFile(source, token.Latest)
	checker.Check(file, source, nil)
	want := []string{
		"Category::query()",
		"Category::query()->where('name', 1)",
		"Category::query()",
	}
	if strings.Join(receivers, "|") != strings.Join(want, "|") {
		t.Errorf("receivers: want %q, got %q", want, receivers)
	}
}

func TestInvalidBuilderArgReadsArgumentsFromTheTree(t *testing.T) {
	checker := setupBuilderArgRule()
	source := `<?php
use App\Models\Category;
Category::query()->select(['name', 'nope', 'name' => 'alias'])->with(['products' => fn ($q) => $q, 'gone']);
Category::with(
    'missingOnNextLine');
Category::withSum('products', 'cost')->get("absent")->where($column)->select();
// Category::where('commented');
$s = "Category::where('quoted')";
`
	var got []string
	for _, f := range checker.Check(parser.ParseFile(source, token.Latest), source, nil) {
		got = append(got, fmt.Sprintf("%d:%d-%d %s", f.StartLine, f.StartCol, f.EndCol, f.Message))
	}
	want := []string{
		"2:36-40 Unknown column 'nope' on model 'Category'",
		"2:100-104 Unknown relation 'gone' on model 'Category'",
		"5:31-35 Unknown column 'cost' on related model 'Product'",
		"5:44-50 Unknown column 'absent' on model 'Category'",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s", strings.Join(got, "\n"))
	}
}
