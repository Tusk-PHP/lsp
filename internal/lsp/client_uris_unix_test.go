//go:build !windows

package lsp

import "testing"

func TestAFileURIOnAnotherHostIsIndexedUnderTheClientsURIUntilClosed(t *testing.T) {
	assertIndexedUnderTheClientsURIUntilClosed(t, "file://server/share/Scratch.php")
}
