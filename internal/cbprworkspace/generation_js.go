//go:build js

package cbprworkspace

import "os"

func publishGenerationDirectory(stage, target string) error { return os.Rename(stage, target) }
func syncGenerationDirectory(string) error { return nil }
