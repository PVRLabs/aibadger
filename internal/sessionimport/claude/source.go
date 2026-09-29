package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PVRLabs/aibadger/internal/sessionimport"
)

const (
	maxProjectEntries = 512
	maxSessionEntries = 2048
	maxInspections    = 128
	maxCandidates     = 40
	metadataBytes     = 64 * 1024
)

var idPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Source struct {
	Root string // Claude's projects directory; injectable in tests.
}

func ValidID(id string) bool { return idPattern.MatchString(id) }

func Default() (Source, error) {
	config := os.Getenv("CLAUDE_CONFIG_DIR")
	if config == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Source{}, fmt.Errorf("Claude home directory unavailable: %w", err)
		}
		config = filepath.Join(home, ".claude")
	}
	if !filepath.IsAbs(config) {
		absolute, err := filepath.Abs(config)
		if err != nil {
			return Source{}, fmt.Errorf("resolving Claude config directory: %w", err)
		}
		config = absolute
	}
	return Source{Root: filepath.Join(config, "projects")}, nil
}

func (s Source) validate() error {
	if !filepath.IsAbs(s.Root) {
		return fmt.Errorf("Claude projects directory is unavailable")
	}
	return nil
}

func names(path string, limit int) ([]string, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	out, err := f.Readdirnames(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	sort.Strings(out)
	return out, more, nil
}

func regular(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func directory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

type record struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	CWD         string          `json:"cwd"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

func summary(path, id, projectRoot string) (sessionimport.Summary, bool) {
	if !regular(path) {
		return sessionimport.Summary{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return sessionimport.Summary{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return sessionimport.Summary{}, false
	}
	out := sessionimport.Summary{ID: id, StartedAt: info.ModTime(), Label: "Claude session"}
	foundTimestamp := false
	r := bufio.NewReader(io.LimitReader(f, metadataBytes))
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		var row record
		if json.Unmarshal(line, &row) != nil || (row.SessionID != "" && !strings.EqualFold(row.SessionID, id)) {
			continue
		}
		if out.WorkingDirectory == "" && filepath.IsAbs(row.CWD) {
			out.WorkingDirectory = row.CWD
		}
		if at, err := time.Parse(time.RFC3339Nano, row.Timestamp); !foundTimestamp && err == nil && !at.IsZero() {
			out.StartedAt = at
			foundTimestamp = true
		}
		if role, content := conversationRow(row); role == "user" && content != "" {
			out.Label = compactLabel(content)
			break
		}
	}
	if projectRoot != "" && filepath.IsAbs(out.WorkingDirectory) {
		rel, err := filepath.Rel(filepath.Clean(projectRoot), filepath.Clean(out.WorkingDirectory))
		out.ProjectMatch = err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return out, true
}

func compactLabel(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 72 {
		return string(r[:72]) + "…"
	}
	return s
}

// List inspects bounded directory entries and transcript prefixes. Claude's
// project directory names are opaque; the recorded cwd supplies association.
func (s Source) List(projectRoot string) ([]sessionimport.Summary, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	projects, _, err := names(s.Root, maxProjectEntries)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		path, id string
		modified time.Time
	}
	var candidates []candidate
	remaining := maxSessionEntries
	for _, project := range projects {
		path := filepath.Join(s.Root, project)
		if !directory(path) {
			continue
		}
		files, _, err := names(path, remaining)
		if err != nil {
			continue
		} // a project may disappear during discovery
		remaining -= len(files)
		for _, file := range files {
			id := strings.TrimSuffix(file, ".jsonl")
			if !strings.HasSuffix(file, ".jsonl") || !ValidID(id) {
				continue
			}
			full := filepath.Join(path, file)
			info, err := os.Lstat(full)
			if err == nil && info.Mode().IsRegular() {
				candidates = append(candidates, candidate{full, id, info.ModTime()})
			}
		}
		if remaining == 0 {
			break
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].modified.After(candidates[j].modified) })
	if len(candidates) > maxInspections {
		candidates = candidates[:maxInspections]
	}
	var out []sessionimport.Summary
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c.id] {
			continue
		}
		if item, ok := summary(c.path, c.id, projectRoot); ok {
			out = append(out, item)
			seen[c.id] = true
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectMatch != out[j].ProjectMatch {
			return out[i].ProjectMatch
		}
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.After(out[j].StartedAt)
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out, nil
}

// Resolve checks only the requested filename in each bounded project directory.
// If an ID appears in more than one project, use the newest regular file.
func (s Source) Resolve(id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("invalid Claude session ID %q", id)
	}
	if err := s.validate(); err != nil {
		return "", err
	}
	projects, more, err := names(s.Root, maxProjectEntries)
	if err != nil {
		return "", err
	}
	var newest string
	var newestModTime time.Time
	for _, project := range projects {
		path := filepath.Join(s.Root, project)
		if !directory(path) {
			continue
		}
		file := filepath.Join(path, id+".jsonl")
		info, err := os.Lstat(file)
		if err == nil && info.Mode().IsRegular() && (newest == "" || info.ModTime().After(newestModTime)) {
			newest = file
			newestModTime = info.ModTime()
		}
	}
	if newest != "" {
		return newest, nil
	}
	if more {
		return "", sessionimport.ErrLookupIncomplete
	}
	return "", sessionimport.ErrNotFound
}
