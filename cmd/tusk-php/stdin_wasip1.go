//go:build wasip1

package main

import (
	"io"
	"syscall"
	"time"
)

// How long to wait for input before asking again. It starts short so requests
// feel instant while typing, and backs off so an idle server stays cheap.
const (
	stdinPollMin = time.Millisecond
	stdinPollMax = 50 * time.Millisecond
)

// stdinReader reads stdin without Go's poller, which can't wait on the pipe VS
// Code's WASI host provides. Sleeping between reads lets background goroutines
// such as indexing run, since wasip1 has only one thread.
type stdinReader struct{}

func (stdinReader) Read(p []byte) (int, error) {
	wait := stdinPollMin
	for {
		n, err := syscall.Read(0, p)
		switch {
		case err == syscall.EAGAIN:
			time.Sleep(wait)
			wait = min(wait*2, stdinPollMax)
		case err != nil:
			return n, err
		case n == 0 && len(p) > 0:
			return 0, io.EOF
		default:
			return n, nil
		}
	}
}

func serverStdin() io.Reader {
	_ = syscall.SetNonblock(0, true)
	return stdinReader{}
}
