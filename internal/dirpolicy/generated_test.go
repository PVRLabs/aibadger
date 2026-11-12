package dirpolicy

import (
	"strings"
	"testing"
)

func TestGeneratedComponents(t *testing.T) {
	for _, name := range GeneratedNames() {
		if !SkipGenerated(name) || !SkipGenerated(strings.ToUpper(name)) {
			t.Errorf("generated directory not excluded: %s", name)
		}
		if SkipGenerated("my" + name + "notes") {
			t.Errorf("substring exclusion: %s", name)
		}
	}
	for _, name := range []string{"docs", "tests", "test", "fixtures", "examples", "articles", "templates", "public", "static", "assets", "scripts", ".github", ".vscode", ".storybook", ".yarn", ".mvn", ".config", "cache", "logs", "reports", "generated"} {
		if SkipGenerated(name) {
			t.Errorf("authored/configuration directory excluded: %s", name)
		}
	}
	if !SkipGenerated("cmake-build-debug") {
		t.Fatal("CMake variant retained")
	}
}
