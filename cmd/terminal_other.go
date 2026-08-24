// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

//go:build !unix

package cmd

// controllingTerminalWidth has no portable equivalent outside Unix. Windows and
// WebAssembly fall through to the standard-stream probe in getTerminalWidth.
func controllingTerminalWidth() int { return 0 }
