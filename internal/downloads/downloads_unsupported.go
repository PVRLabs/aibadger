//go:build !linux && !darwin && !windows

package downloads

import "fmt"

func nativeDownloadsDirectory() (string, error) {
	return "", fmt.Errorf("Downloads directory is not supported on this platform")
}
