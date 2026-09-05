// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Package atomicfile publishes complete files without exposing a truncated
// destination to concurrent readers.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Write persists data to a temporary file in the destination directory and
// atomically replaces the destination only after the complete file is synced.
func Write(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temporary file for %s: %w", filepath.Base(path), err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("protecting temporary file for %s: %w", filepath.Base(path), err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("writing temporary file for %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("syncing temporary file for %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary file for %s: %w", filepath.Base(path), err)
	}
	if err := replace(tmpPath, path); err != nil {
		return fmt.Errorf("publishing %s: %w", filepath.Base(path), err)
	}
	if err := syncDirectory(dir); err != nil {
		return fmt.Errorf("syncing directory for %s: %w", filepath.Base(path), err)
	}
	return nil
}
