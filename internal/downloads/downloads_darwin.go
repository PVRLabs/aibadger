//go:build darwin

package downloads

import (
	"fmt"
	"os"
	"path/filepath"
)

func nativeDownloadsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find user home directory: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("find user home directory: home is empty")
	}
	return filepath.Join(home, "Downloads"), nil
}
