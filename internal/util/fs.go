package util

import "os"

// FileExists returns true if the path exists and is not a directory.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// FileExistsAndNotDir returns true if the path exists and is not a directory.
// It returns an error if os.Stat fails for reasons other than non-existence.
func FileExistsAndNotDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}
