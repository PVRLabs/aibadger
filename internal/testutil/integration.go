// Package testutil provides shared test helpers.
package testutil

import (
	"os"
	"testing"
)

// RequireIntegration skips tests that use real Git repositories unless explicitly enabled.
func RequireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("BADGER_INTEGRATION") != "1" {
		t.Skip("Git integration test: set BADGER_INTEGRATION=1 to run")
	}
}
