// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

//go:build unix

package cmd

import (
	"os"
	"syscall"
	"unsafe"
)

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// controllingTerminalWidth asks the controlling terminal for its width, which
// works even when stdout is redirected. Returns 0 when there is no terminal.
func controllingTerminalWidth() int {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	var ws winsize
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	); errno == 0 && ws.Col > 0 {
		return int(ws.Col)
	}
	return 0
}
