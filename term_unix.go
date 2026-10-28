//go:build darwin || linux

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func termRows() int {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Row == 0 {
		return 24
	}
	return int(ws.Row)
}

func termCols() int {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0
	}
	return int(ws.Col)
}

func termSetup()   {}
func termRestore() {}
