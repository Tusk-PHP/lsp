package parser

import (
	"reflect"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestFileNodeDefines(t *testing.T) {
	file := ParseFile(`<?php
namespace App;
define('APP_VERSION', '1.2');
\define("App\\Config\\DEBUG", true);
DEFINE('LOWER', 1 + 2);
define(value: [1, 2], constant_name: 'NAMED');
if (!defined('GUARDED')) {
    define('GUARDED', __DIR__ . '/x');
}
function boot() { define('IN_FUNCTION', 1); }
class K { public function m() { define('IN_METHOD', null); } }
$f = fn() => define('IN_ARROW', 2);
define($name, 1);
define("PREFIX_$name", 1);
define('has space', 1);
define('\LEADING', 1);
define('1DIGIT', 1);
define('ONLY_NAME');
define(...$args);
$o->define('METHOD_CALL', 1);
K::define('STATIC_CALL', 1);
Other\define('QUALIFIED', 1);
`, token.Latest)
	type got struct {
		Name, FullName, Value string
		Line, Col             int
	}
	var defines []got
	for _, d := range file.Defines {
		defines = append(defines, got{d.Name, d.FullName, d.Value, d.StartLine, d.StartCol})
	}
	want := []got{
		{"APP_VERSION", "APP_VERSION", "'1.2'", 2, 8},
		{"DEBUG", `App\Config\DEBUG`, "true", 3, 22},
		{"LOWER", "LOWER", "1+2", 4, 8},
		{"NAMED", "NAMED", "[1,2]", 5, 38},
		{"GUARDED", "GUARDED", "__DIR__.'/x'", 7, 12},
		{"IN_FUNCTION", "IN_FUNCTION", "1", 9, 26},
		{"IN_METHOD", "IN_METHOD", "null", 10, 40},
		{"IN_ARROW", "IN_ARROW", "2", 11, 21},
	}
	if !reflect.DeepEqual(defines, want) {
		t.Fatalf("defines:\n got %+v\nwant %+v", defines, want)
	}
	if len(file.Constants) != 0 {
		t.Fatalf("define() calls are not const declarations: %+v", file.Constants)
	}
}

func TestFileNodeDefineThroughAnImportedFunction(t *testing.T) {
	file := ParseFile(`<?php
namespace App;
use function Lib\define;
use function define as declare_constant;
define('NOT_GLOBAL', 1);
declare_constant('ALIASED', 1);
`, token.Latest)
	if len(file.Defines) != 1 || file.Defines[0].FullName != "ALIASED" {
		t.Fatalf("only the call that reaches PHP's define() declares a constant: %+v", file.Defines)
	}
}

func TestFileNodeConstHasItsNamespacedFullName(t *testing.T) {
	file := ParseFile("<?php\nnamespace App;\nconst A = 1, B = 2;\n", token.Latest)
	if len(file.Constants) != 2 || file.Constants[0].FullName != `App\A` || file.Constants[1].FullName != `App\B` {
		t.Fatalf("const declares constants in the file's namespace: %+v", file.Constants)
	}
}
