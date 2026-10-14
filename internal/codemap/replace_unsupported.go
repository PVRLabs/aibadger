//go:build !darwin && !linux && !windows

package codemap

import (
	"fmt"
	"runtime"
)

func replaceReport(source, destination string) error {
	return fmt.Errorf("atomic report publication is unsupported on %s", runtime.GOOS)
}
