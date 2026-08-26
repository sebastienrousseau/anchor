// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Command gen-registry regenerates AskISO's embedded message registry.
//
// \tgo generate ./internal/registry/
package main

import (
	"os"

	"github.com/sebastienrousseau/askiso/internal/registrygen"
)

func main() {
	os.Exit(registrygen.Run(os.Args[1:], os.Stdout, os.Stderr))
}
