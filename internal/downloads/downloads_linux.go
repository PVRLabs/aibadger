//go:build linux

package downloads

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func nativeDownloadsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find user home directory: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("find user home directory: home is empty")
	}

	configHome := xdgConfigHome(home, os.Getenv("XDG_CONFIG_HOME"))
	configPath := filepath.Join(configHome, "user-dirs.dirs")
	file, err := os.Open(configPath)
	if err != nil {
		return "", fmt.Errorf("read XDG user-directories configuration %q: %w", configPath, err)
	}
	defer file.Close()

	return parseXDGDownloadsConfig(file, home)
}

func xdgConfigHome(home, configured string) string {
	if filepath.IsAbs(configured) {
		return configured
	}
	return filepath.Join(home, ".config")
}

func parseXDGDownloadsConfig(reader io.Reader, home string) (string, error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "XDG_DOWNLOAD_DIR=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "XDG_DOWNLOAD_DIR="))
		value = strings.Trim(value, "\"")
		value = strings.ReplaceAll(value, "$HOME", home)
		if value == "" {
			return "", fmt.Errorf("XDG user-directories configuration has an empty XDG_DOWNLOAD_DIR")
		}
		return value, nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read XDG user-directories configuration: %w", err)
	}
	return "", fmt.Errorf("XDG user-directories configuration has no XDG_DOWNLOAD_DIR")
}
