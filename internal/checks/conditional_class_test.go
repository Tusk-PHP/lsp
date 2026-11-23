package checks

import (
	"strings"
	"testing"
)

// A class declared under `if (false)` or `if (!class_exists())` is a placeholder
// or polyfill: the class code runs with may have more members.
func TestUnknownMemberQuietOnConditionalClass(t *testing.T) {
	placeholder := `<?php
namespace App;
class_alias('App\RealListener', 'App\Listener');
if (false) {
    class Listener {}
}
`
	polyfill := `<?php
namespace App {
    if (!class_exists(Client::class, false)) {
        class Client { public function connect(): void {} }
    }
}
`
	plain := `<?php
namespace App;
class Post { public function publish(): void {} }
`
	cases := []struct {
		name, typ, use string
		want           []string
	}{
		{"placeholder under if (false)", `App\Listener`, `$v->globalListenerDisabled();`, nil},
		{"polyfill under if (!class_exists())", `App\Client`, `$v->getCollection();`, nil},
		{"an ordinary class is still checked", `App\Post`, `$v->missing();`, []string{"Unknown member 'missing' on 'Post'"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := "<?php\nnamespace App\\Http;\n" + tc.use + "\n"
			got := unknownMemberMessages(checkUnknownMembers(t, map[string]string{"$v": tc.typ}, placeholder, polyfill, plain, source))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A subclass declared inside a block, one of several variants, doesn't make
// `$this` in its parent answer for every member.
func TestUnknownMemberConditionalSubclassOfThis(t *testing.T) {
	base := `<?php
class ftp_base {
    var $Verbose;
    function run() {
        $this->verbose = true;
        $this->my_regexp('x');
        $this->Verbose = true;
        $this->send();
    }
}
`
	variants := `<?php
class ftp_sockets extends ftp_base { function send() {} }
if ($c) {
    class ftp extends ftp_sockets {}
} else {
    class ftp extends ftp_sockets { function extra() {} }
}
`
	got := unknownMemberMessages(checkUnknownMembers(t, map[string]string{"$this": "ftp_base"}, variants, base))
	want := []string{"Unknown member 'verbose' on 'ftp_base'", "Unknown member 'my_regexp' on 'ftp_base'"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}
