package filekind

import (
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestClassifyCppExtensionsAsSource(t *testing.T) {
	for _, name := range []string{"parser.cc", "parser.cxx", "parser.hh", "parser.hxx"} {
		t.Run(filepath.Ext(name), func(t *testing.T) {
			if got := Classify(name); got != model.FileKindSource {
				t.Fatalf("Classify(%q) = %q, want %q", name, got, model.FileKindSource)
			}
		})
	}
}
