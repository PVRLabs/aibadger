//go:build !windows

package downloads

import "os"

func replacePromptFile(source, destination string) error {
	return os.Rename(source, destination)
}
