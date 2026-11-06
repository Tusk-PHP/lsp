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
