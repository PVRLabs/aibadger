package pathmatch

import (
	"strings"
	"testing"
)

func TestMatchingContract(t *testing.T) {
	for _, tc := range []struct {
		pattern, name string
		want          bool
	}{
		{"vendor", "vendor", true}, {"vendor", "src/vendor/app.go", true}, {"vendor", "src/vendor.go", false},
		{"Dockerfile", "a/Dockerfile", true}, {"Dockerfile", "a/dockerfile", false},
		{"public/assets/**", "public/assets", true}, {"public/assets/**", "public/assets/a.js", true},
		{"public/assets/**", "other/public/assets/a.js", false},
		{"src/generated", "src/generated/a.go", true}, {"src/generated", "other/src/generated/a.go", false},
		{"**/*.min.js", "app.min.js", true}, {"**/*.min.js", "a/b/app.min.js", true},
		{"a/**/b/**/c", "a/b/c", true}, {"a/**/b/**/c", "a/x/b/y/z/c/file", true},
		{"a/*/c", "a/c", false}, {"a/*/c", "a/b/c", true}, {"a/*/c", "a/b/d/c", false},
		{"*.csv", "a/data.csv/child", true}, {"**", "anything", true},
		{"日本 語", "a/日本 語/file", true}, {"a'b $();&{}!", "a'b $();&{}!/file", true},
		{"!foo", "foo", false}, {"!foo", "!foo", true}, {"{a,b}", "a", false},
		{" ", "a/ /file", true}, {"a*", "a", true},
	} {
		t.Run(tc.pattern+":"+tc.name, func(t *testing.T) {
			if err := Validate([]string{tc.pattern}); err != nil {
				t.Fatal(err)
			}
			if got := Matches([]string{tc.pattern}, tc.name); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	if Matches(nil, "file") {
		t.Fatal("empty patterns matched")
	}
	if !Matches([]string{"missing", "vendor", "vendor"}, "src/vendor/file") {
		t.Fatal("additive patterns did not match")
	}
}

func TestValidation(t *testing.T) {
	for _, pattern := range []string{"", "/a", "//host/share", "C:foo", "c:/foo", "Z:", "C:\\foo", "\\\\host\\share",
		"a/", "a//b", "./a", "a/../b", "a/./b", "a?", "a[bc]", "a]", "a\\b", "a**b", "***", "**x", "x**", "a\nb", "a\x00b", "a\u0085b"} {
		if err := Validate([]string{pattern}); err == nil || !strings.Contains(err.Error(), "invalid --exclude pattern") {
			t.Errorf("%q: %v", pattern, err)
		}
	}
	for _, pattern := range []string{"a", "**", "a/**/b", "**/**", "a*b", "a'b $();&{}", "日本 語", " a ", "-foo", "</script>"} {
		if err := Validate([]string{pattern, pattern}); err != nil {
			t.Errorf("%q: %v", pattern, err)
		}
	}
}
