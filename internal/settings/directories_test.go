package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadDirectoryFixture(t *testing.T, body string) (Settings, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return LoadSettings(path)
}

func TestDirectorySettingsPresenceAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body      string
		enabled, custom bool
		warnings        bool
	}{
		{"defaults", `{}`, true, false, false},
		{"disabled", `{"exclude_generated_directories":false}`, false, false, false},
		{"additional", `{"additional_excluded_directories":[".my-cache","custom-output"]}`, true, true, false},
		{"independent", `{"exclude_generated_directories":false,"additional_excluded_directories":[".my-cache"]}`, false, true, false},
		{"invalid toggle", `{"exclude_generated_directories":"false","additional_excluded_directories":[".my-cache"],"whitespace_mode":"ignore"}`, true, true, true},
		{"invalid list", `{"exclude_generated_directories":false,"additional_excluded_directories":"cache","whitespace_mode":"ignore"}`, false, false, true},
		{"invalid entries", `{"exclude_generated_directories":false,"additional_excluded_directories":[".my-cache","../bad","!negate","*glob",42,null,"nested/path","",".."],"whitespace_mode":"ignore"}`, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := loadDirectoryFixture(t, tc.body)
			if err != nil {
				t.Fatal(err)
			}
			p := s.DirectoryPolicy()
			if p.Skip(".playwright-cli") != tc.enabled || p.Skip(".MY-CACHE") != tc.custom {
				t.Fatalf("effective policy mismatch: %+v", s)
			}
			if (len(s.Warnings) > 0) != tc.warnings {
				t.Fatalf("warnings: %v", s.Warnings)
			}
			if strings.Contains(tc.body, "whitespace_mode") && s.WhitespaceMode != "ignore" {
				t.Fatal("unrelated setting lost")
			}
		})
	}
	missing, err := loadDirectoryFixture(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := loadDirectoryFixture(t, `{"exclude_generated_directories":false,"additional_excluded_directories":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if missing.ExcludeGeneratedDirectories != nil || missing.AdditionalExcludedDirectories != nil {
		t.Fatal("omitted keys lost presence")
	}
	if explicit.ExcludeGeneratedDirectories == nil || *explicit.ExcludeGeneratedDirectories || explicit.AdditionalExcludedDirectories == nil || len(*explicit.AdditionalExcludedDirectories) != 0 {
		t.Fatal("explicit false/empty lost presence")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := SaveSettings(path, explicit); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"exclude_generated_directories": false`) || !strings.Contains(string(data), `"additional_excluded_directories": []`) {
		t.Fatalf("presence lost saving: %s", data)
	}
}

func TestDirectorySettingsSurviveInvalidLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"exclude_generated_directories":false,"additional_excluded_directories":["custom-output"],"limits":"invalid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	p, warnings := LoadDirectoryPolicy(path)
	if p.Skip("build") || !p.Skip("CUSTOM-OUTPUT") || len(warnings) == 0 {
		t.Fatalf("valid policy lost with invalid limits: %v", warnings)
	}
}
