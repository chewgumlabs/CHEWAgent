//go:build darwin

package main

import "syscall"

const (
	ioctlReadTermios  = uintptr(syscall.TIOCGETA)
	ioctlWriteTermios = uintptr(syscall.TIOCSETA)
)
