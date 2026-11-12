package settings

import (
	"fmt"
	"github.com/PVRLabs/aibadger/internal/defaults"
	"os"
	"path/filepath"
)

// LoadProject overlays project settings on user settings without rewriting either file.
// Zero and invalid limits inherit the preceding layer.
func LoadProject(userPath, root string) (Settings, []string) {
	return loadLayers(Settings{}, []string{userPath, filepath.Join(root, settingsDirName, "settings.json")})
}

// OverlayProject applies just the repository layer to already loaded user settings.
func OverlayProject(base Settings, root string) (Settings, []string) {
	return loadLayers(base, []string{filepath.Join(root, settingsDirName, "settings.json")})
}

func loadLayers(result Settings, paths []string) (Settings, []string) {
	if result.Limits != nil {
		copied := *result.Limits
		result.Limits = &copied
	}
	var warnings []string
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[filepath.Clean(path)] {
			continue
		}
		seen[filepath.Clean(path)] = true
		layer, err := LoadSettings(path)
		if err != nil && !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("Could not fully load settings from %s: %v", path, err))
		}
		warnings = append(warnings, layer.Warnings...)
		if layer.ExcludeGeneratedDirectories != nil {
			result.ExcludeGeneratedDirectories = layer.ExcludeGeneratedDirectories
		}
		if layer.AdditionalExcludedDirectories != nil {
			result.AdditionalExcludedDirectories = layer.AdditionalExcludedDirectories
		}
		if layer.WhitespaceMode != "" {
			result.WhitespaceMode = layer.WhitespaceMode
		}
		if layer.Limits == nil {
			continue
		}
		if result.Limits == nil {
			result.Limits = &SettingsLimits{}
		}
		MergeLimits(result.Limits, layer.Limits, func(message string) { warnings = append(warnings, path+": "+message) })
	}
	return result, warnings
}

// MergeLimits validates each supplied key independently.
func MergeLimits(dst, src *SettingsLimits, warn func(string)) {
	specs := []struct {
		key             string
		value, min, max int
		target          *int
	}{
		{"max_top_files_per_package", src.MaxTopFilesPerPackage, 3, 50, &dst.MaxTopFilesPerPackage},
		{"max_files_per_directory", src.MaxFilesPerDirectory, defaults.MaxFilesPerDirectory, defaults.MaxFilesPerDirectoryUpperBound, &dst.MaxFilesPerDirectory},
		{"max_context_file_bytes", src.MaxContextFileBytes, defaults.MaxContextFileBytes, defaults.MaxContextFileBytesUpperBound, &dst.MaxContextFileBytes},
		{"max_prompt_two_bytes", src.MaxPromptTwoBytes, defaults.MaxPromptTwoBytes, defaults.MaxPromptTwoBytesUpperBound, &dst.MaxPromptTwoBytes},
		{"max_topology_prompt_bytes", src.MaxTopologyPromptBytes, defaults.MaxTopologyPromptBytes, defaults.MaxTopologyPromptBytesUpperBound, &dst.MaxTopologyPromptBytes},
	}
	for _, spec := range specs {
		if spec.value == 0 {
			continue
		}
		if spec.value < spec.min || spec.value > spec.max {
			if warn != nil {
				warn(fmt.Sprintf("settings limits.%s must be between %d and %d; keeping the current value", spec.key, spec.min, spec.max))
			}
			continue
		}
		*spec.target = spec.value
	}
}
