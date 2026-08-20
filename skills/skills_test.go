package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefinitionsAreCanonicalAndDeterministic(t *testing.T) {
	defs := Definitions()
	wantNames := []string{"badger-review", "badger-handoff"}
	if len(defs) != len(wantNames) {
		t.Fatalf("Definitions() length = %d, want %d", len(defs), len(wantNames))
	}

	for i, wantName := range wantNames {
		if defs[i].Name != wantName {
			t.Errorf("Definitions()[%d].Name = %q, want %q", i, defs[i].Name, wantName)
		}
		path := filepath.Join(defs[i].Name, "SKILL.md")
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if defs[i].Content != string(want) {
			t.Errorf("Definitions()[%d].Content does not match %s", i, path)
		}
	}

	second := Definitions()
	if !reflect.DeepEqual(defs, second) {
		t.Fatal("Definitions() is not deterministic")
	}
	defs[0].Content = "changed"
	if Definitions()[0].Content == "changed" {
		t.Fatal("Definitions() returned mutable internal state")
	}
}
