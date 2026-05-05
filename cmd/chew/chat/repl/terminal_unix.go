//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// repairTerminalViewport undoes stale ANSI scroll regions left by older
// experimental builds. It does not clear the screen or move text around.
func repairTerminalViewport() {
	if !stdoutIsTerminal() {
		return
	}
	fmt.Print("\033[r")
}

func shouldRunTUI() bool {
	if envFlagSet("CHEW_PLAIN") || envFlagSet("CHEW_NO_TUI") {
		return false
	}
	return stdinIsTerminal() && stdoutIsTerminal()
}

func envFlagSet(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return v != "" && v != "0" && v != "false" && v != "no"
}

type rawTerminal struct {
	fd    int
	state syscall.Termios
}

func enterRawTerminal() (*rawTerminal, error) {
	fd := int(os.Stdin.Fd())
	var old syscall.Termios
	if err := ioctlTermios(fd, ioctlReadTermios, &old); err != nil {
		return nil, err
	}

	raw := old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	if err := ioctlTermios(fd, ioctlWriteTermios, &raw); err != nil {
		return nil, err
	}
	return &rawTerminal{fd: fd, state: old}, nil
}

func (r *rawTerminal) restore() {
	if r == nil {
		return
	}
	_ = ioctlTermios(r.fd, ioctlWriteTermios, &r.state)
}

func stdinIsTerminal() bool {
	return fdIsTerminal(int(os.Stdin.Fd()))
}

func stdoutIsTerminal() bool {
	return fdIsTerminal(int(os.Stdout.Fd()))
}

func fdIsTerminal(fd int) bool {
	var t syscall.Termios
	return ioctlTermios(fd, ioctlReadTermios, &t) == nil
}

func ioctlTermios(fd int, request uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), request, uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return errno
	}
	return nil
}

func terminalSize() (int, int) {
	var ws struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.Col == 0 || ws.Row == 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}
