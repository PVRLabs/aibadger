//go:build windows

package downloads

import "golang.org/x/sys/windows"

func replacePromptFile(source, destination string) error {
	return windows.Rename(source, destination)
}
