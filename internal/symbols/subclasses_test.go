package symbols

import (
	"sort"
	"testing"
)

func TestSubclassesFollowsReverseInheritance(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///base.php", "<?php\nclass ftp_base {}\nclass Other {}\n")
	idx.IndexFile("file:///sockets.php", "<?php\nclass ftp_sockets extends FTP_BASE {}\n")
	idx.IndexFile("file:///pure.php", "<?php\nclass ftp_pure extends ftp_base {}\nclass ftp extends ftp_pure {}\n")

	got := idx.Subclasses("ftp_base")
	sort.Strings(got)
	if want := []string{"ftp", "ftp_pure", "ftp_sockets"}; !equalStrings(got, want) {
		t.Errorf("Subclasses(ftp_base) = %v, want %v", got, want)
	}
	if got := idx.Subclasses("Other"); len(got) != 0 {
		t.Errorf("Other has no subclasses, got %v", got)
	}

	// Re-indexing a file without the subclass drops it.
	idx.IndexFile("file:///sockets.php", "<?php\n")
	got = idx.Subclasses("ftp_base")
	sort.Strings(got)
	if want := []string{"ftp", "ftp_pure"}; !equalStrings(got, want) {
		t.Errorf("after removing ftp_sockets: %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestImplementations(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///contracts.php", `<?php
namespace Lib;
interface Item {}
`)
	idx.IndexFile("file:///order.php", `<?php
namespace Lib;
interface OrderItem extends Item {}
class OrderLine implements OrderItem {}
class BundleLine extends OrderLine {}
class Unrelated {}
`)
	got := idx.Implementations("Lib\\Item")
	sort.Strings(got)
	if want := []string{"Lib\\BundleLine", "Lib\\OrderItem", "Lib\\OrderLine"}; !equalStrings(got, want) {
		t.Errorf("Implementations(Item) = %v, want %v", got, want)
	}

	// Re-indexing a file without the sub-interface drops it and what's below it.
	idx.IndexFile("file:///order.php", "<?php\n")
	if got := idx.Implementations("Lib\\Item"); len(got) != 0 {
		t.Errorf("after removing order.php: %v, want none", got)
	}
}
