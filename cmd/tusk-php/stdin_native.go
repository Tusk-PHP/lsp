//go:build !wasip1

package main

import (
	"io"
	"os"
)

func serverStdin() io.Reader { return os.Stdin }
