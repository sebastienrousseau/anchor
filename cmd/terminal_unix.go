// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

//go:build unix

package cmd

import (
	"os"

	"golang.org/x/sys/unix"
)

// controllingTerminalWidth asks the controlling terminal for its width, which
// works even when stdout is redirected. Returns 0 when there is no terminal.
func controllingTerminalWidth() int {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()

	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err == nil && ws.Col > 0 {
		return int(ws.Col)
	}
	return 0
}
