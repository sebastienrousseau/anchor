//go:build js

package cbprworkspace

import "os"

func lockPublicationFile(*os.File) error { return nil }
func unlockPublicationFile(*os.File) error { return nil }
