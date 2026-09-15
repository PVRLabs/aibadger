package filekind

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestClassifyCppExtensionsAsSource(t *testing.T) {
	for _, name := range []string{"parser.cc", "parser.cxx", "parser.hh", "parser.hxx"} {
		t.Run(filepath.Ext(name), func(t *testing.T) {
			if got := classifyByName(name); got != model.FileKindSource {
				t.Fatalf("classifyByName(%q) = %q, want %q", name, got, model.FileKindSource)
			}
		})
	}
}

func TestClassifyScriptLanguageAliasesAsSource(t *testing.T) {
	for _, name := range []string{"module.mts", "module.cts", "build.kts"} {
		t.Run(filepath.Ext(name), func(t *testing.T) {
			if got := classifyByName(name); got != model.FileKindSource {
				t.Fatalf("classifyByName(%q) = %q, want %q", name, got, model.FileKindSource)
			}
		})
	}
}

func TestClassifyLegacyAndSpecializedExtensionsAsSource(t *testing.T) {
	for _, name := range []string{
		"unit.ads", "body.adb", "legacy.ada", "program.cbl", "program.cob", "program.cobol", "copy.ccp", "copy.cpy",
		"job.jcl", "design.sv", "design.svh", "design.vhd", "design.vhdl",
		"fixed.f77", "free.f90", "free.f95", "free.f03", "free.f08", "free.fpp", "free.ftn",
		"source.pli", "source.pl1", "program.rpgle", "program.sqlrpgle", "copy.rpgleinc", "program.sqlrpg",
		"command.clle", "command.clp", "command.clp38", "report.abap",
	} {
		t.Run(filepath.Ext(name), func(t *testing.T) {
			if got := classifyByName(name); got != model.FileKindSource {
				t.Fatalf("classifyByName(%q) = %q, want %q", name, got, model.FileKindSource)
			}
		})
	}
}

func TestClassifyUnknownTextByContent(t *testing.T) {
	root := t.TempDir()
	textPath := filepath.Join(root, "notes.xyz")
	if err := os.WriteFile(textPath, []byte("plain text\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := Classify(textPath); got != model.FileKindSource {
		t.Fatalf("Classify(%q) = %q, want content-sniffed source", textPath, got)
	}
}
