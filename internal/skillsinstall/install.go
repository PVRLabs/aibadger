// Package skillsinstall installs Badger's bundled official Agent Skills.
package skillsinstall

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/PVRLabs/aibadger/skills"
)

const (
	installDirMode  = 0o700
	installFileMode = 0o600
)

var officialSkillOrder = []string{"handoff", "badger-review"}

// Install atomically installs the supplied official definitions below
// skillsRoot. It does not inspect or remove unrelated files.
func Install(skillsRoot string, definitions []skills.Definition, stdout, stderr io.Writer) error {
	ordered, err := validateDefinitions(definitions)
	if err != nil {
		return reportError(stderr, err)
	}
	if err := os.MkdirAll(skillsRoot, installDirMode); err != nil {
		return reportError(stderr, fmt.Errorf("create skills root %q: %w", skillsRoot, err))
	}

	for _, definition := range ordered {
		action, target, err := installDefinition(skillsRoot, definition)
		if err != nil {
			return reportError(stderr, fmt.Errorf("install %s at %q: %w", definition.Name, target, err))
		}
		if _, err := fmt.Fprintf(stdout, "%s %s: %s\n", action, definition.Name, target); err != nil {
			return reportError(stderr, fmt.Errorf("report %s at %q: %w", definition.Name, target, err))
		}
	}
	return nil
}

func validateDefinitions(definitions []skills.Definition) ([]skills.Definition, error) {
	if len(definitions) != len(officialSkillOrder) {
		return nil, fmt.Errorf("expected %d official skills, got %d", len(officialSkillOrder), len(definitions))
	}
	byName := make(map[string]skills.Definition, len(definitions))
	for _, definition := range definitions {
		if definition.Name == "" {
			return nil, fmt.Errorf("official skill name is empty")
		}
		if _, exists := byName[definition.Name]; exists {
			return nil, fmt.Errorf("duplicate official skill name %q", definition.Name)
		}
		byName[definition.Name] = definition
	}

	ordered := make([]skills.Definition, 0, len(officialSkillOrder))
	for _, name := range officialSkillOrder {
		definition, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("missing official skill %q", name)
		}
		ordered = append(ordered, definition)
	}
	return ordered, nil
}

func installDefinition(skillsRoot string, definition skills.Definition) (action, target string, err error) {
	dir := filepath.Join(skillsRoot, definition.Name)
	target = filepath.Join(dir, "SKILL.md")
	dirInfo, lstatErr := os.Lstat(dir)
	switch {
	case lstatErr == nil && dirInfo.Mode()&os.ModeSymlink != 0:
		return "", target, fmt.Errorf("skill directory is a symlink")
	case lstatErr == nil && !dirInfo.IsDir():
		return "", target, fmt.Errorf("skill directory is not a directory")
	case lstatErr != nil && !os.IsNotExist(lstatErr):
		return "", target, fmt.Errorf("inspect skill directory: %w", lstatErr)
	case os.IsNotExist(lstatErr):
		if err := os.Mkdir(dir, installDirMode); err != nil {
			return "", target, fmt.Errorf("create skill directory: %w", err)
		}
	}

	action = "installed"
	if targetInfo, err := os.Lstat(target); err == nil {
		if targetInfo.Mode()&os.ModeSymlink != 0 {
			return "", target, fmt.Errorf("SKILL.md target is a symlink")
		}
		if !targetInfo.Mode().IsRegular() {
			return "", target, fmt.Errorf("SKILL.md target is not a regular file")
		}
		action = "updated"
	} else if !os.IsNotExist(err) {
		return "", target, fmt.Errorf("inspect SKILL.md target: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".SKILL.md-*")
	if err != nil {
		return "", target, fmt.Errorf("create atomic temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(installFileMode); err != nil {
		_ = tmp.Close()
		return "", target, fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err := io.WriteString(tmp, definition.Content); err != nil {
		_ = tmp.Close()
		return "", target, fmt.Errorf("write temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", target, fmt.Errorf("sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", target, fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return "", target, fmt.Errorf("replace target atomically: %w", err)
	}
	return action, target, nil
}

func reportError(stderr io.Writer, err error) error {
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, "skills install: %v\n", err)
	}
	return err
}
