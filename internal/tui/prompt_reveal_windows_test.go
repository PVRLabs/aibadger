package tui

import "testing"

func TestExplorerRevealCommand(t *testing.T) {
	for _, path := range []string{`C:\Temp\prompt.txt`, `C:\Users\Jane Doe\prompt.txt`, `C:\Temp\İstanbul →.txt`} {
		cmd, err := explorerRevealCommand("explorer", "/select,"+path)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := cmd.SysProcAttr.CmdLine, `explorer /select,"`+path+`"`; got != want {
			t.Fatalf("command line = %q, want %q", got, want)
		}
	}
}

func TestExplorerRevealMissingExecutable(t *testing.T) {
	if err := runPromptFileReveal(`C:\no-such-badger-opener\explorer.exe`, `/select,C:\Temp\prompt.txt`); err == nil {
		t.Fatal("missing Explorer must fail")
	}
}
