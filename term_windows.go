//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func termRows() int {
	var csbi windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(os.Stdout.Fd()), &csbi); err != nil {
		return 24
	}
	rows := int(csbi.Window.Bottom - csbi.Window.Top + 1)
	if rows < 1 {
		return 24
	}
	return rows
}

var (
	vtMode  uint32
	vtWasOn bool
)

// termSetup включает обработку ANSI-последовательностей в консоли
// (цвет и области прокрутки; Windows 10+ / Windows Terminal).
func termSetup() {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) == nil {
		vtMode, vtWasOn = mode, true
		_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}

func termRestore() {
	if vtWasOn {
		_ = windows.SetConsoleMode(windows.Handle(os.Stdout.Fd()), vtMode)
	}
}
