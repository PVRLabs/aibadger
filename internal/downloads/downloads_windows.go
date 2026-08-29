//go:build windows

package downloads

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func nativeDownloadsDirectory() (string, error) {
	path, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", fmt.Errorf("resolve Downloads Known Folder: %w", err)
	}
	return path, nil
}
