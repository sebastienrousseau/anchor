// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

//go:build !windows && !js

package cbprworkspace

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockPublicationFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

func unlockPublicationFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
