//go:build darwin || linux

package codemap

import "os"

// The staging file is in the destination directory, so this is a same-filesystem
// rename. Publication is atomic; no crash-durability guarantee is implied.
func replaceReport(source, destination string) error { return os.Rename(source, destination) }
