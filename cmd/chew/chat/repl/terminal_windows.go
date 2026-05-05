//go:build windows

package main

import (
	"errors"
	"os"
)

func repairTerminalViewport() {}

func shouldRunTUI() bool {
	return false
}

type rawTerminal struct{}

func enterRawTerminal() (*rawTerminal, error) {
	return nil, errors.New("terminal TUI is not enabled on Windows")
}

func (r *rawTerminal) restore() {}

func stdinIsTerminal() bool {
	return fileIsCharDevice(os.Stdin)
}

func stdoutIsTerminal() bool {
	return fileIsCharDevice(os.Stdout)
}

func fileIsCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func terminalSize() (int, int) {
	return 80, 24
}
