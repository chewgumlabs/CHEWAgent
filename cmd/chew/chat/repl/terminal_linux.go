//go:build linux

package main

import "syscall"

const (
	ioctlReadTermios  = uintptr(syscall.TCGETS)
	ioctlWriteTermios = uintptr(syscall.TCSETS)
)
