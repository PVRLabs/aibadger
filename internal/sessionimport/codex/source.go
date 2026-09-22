package codex

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

// Discovery reads at most 15 date buckets, 1,024 filenames, 128 candidate
// metadata prefixes (64 KiB each), then returns the best 40. Exact lookup has
// no date cutoff but stops after 50,000 directory entries.
const (
	listDays            = 15
	listEntryLimit      = 1024
	listInspectionLimit = 128
	listCandidateLimit  = 40
	metadataReadLimit   = 64 * 1024
	lookupEntryLimit    = 50000
)

var idPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var filenamePattern = regexp.MustCompile(`^rollout-[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}-[0-9]{2}-[0-9]{2}-([0-9a-fA-F-]{36})\.jsonl$`)

type Source struct {
	Root string // The known sessions directory; injectable for isolated tests.
	Now  func() time.Time
	// Lower budgets are used by focused tests; production always uses constants.
	listBudget, inspectionBudget, candidateBudget, lookupBudget int
}

func lowerBudget(requested, ceiling int) int {
	if requested > 0 && requested < ceiling {
		return requested
	}
	return ceiling
}

func Default() (Source, error) {
	home, err := os.UserHomeDir()
	return sourceFromHome(home, err)
}

func sourceFromHome(home string, err error) (Source, error) {
	if err != nil {
		return Source{}, fmt.Errorf("Codex home directory unavailable: %w", err)
	}
	if !filepath.IsAbs(home) {
		return Source{}, fmt.Errorf("Codex home directory is not absolute")
	}
	return Source{Root: filepath.Join(home, ".codex", "sessions")}, nil
}

func ValidID(id string) bool { return idPattern.MatchString(id) }

func (s Source) validateRoot() error {
	if !filepath.IsAbs(s.Root) {
		return fmt.Errorf("Codex sessions root is unavailable")
	}
	return nil
}

func (s Source) today() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func fileID(name string) string {
	m := filenamePattern.FindStringSubmatch(name)
	if len(m) != 2 || !ValidID(m[1]) {
		return ""
	}
	return m[1]
}

func regular(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func directory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

// names returns at most max names. A caller can detect an unfinished directory
// from the extra entry; neither discovery nor lookup loads an unbounded listing.
func names(path string, max int) ([]string, bool, error) {
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
	n, err := f.Readdirnames(max + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	more := len(n) > max
	if more {
		n = n[:max]
	}
	sort.Strings(n)
	return n, more, nil
}

func bucket(root string, day time.Time) string {
	return filepath.Join(root, day.Format("2006"), day.Format("01"), day.Format("02"))
}

type record struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type metadata struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
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
	r := bufio.NewReader(io.LimitReader(f, metadataReadLimit))
	out := sessionimport.Summary{ID: id}
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		var row record
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		if row.Type == "session_meta" {
			var m metadata
			if json.Unmarshal(row.Payload, &m) == nil && (strings.EqualFold(m.ID, id) || strings.EqualFold(m.SessionID, id)) {
				out.WorkingDirectory = m.CWD
				out.StartedAt, _ = time.Parse(time.RFC3339Nano, m.Timestamp)
			}
		}
		if role, text, _ := conversationRow(row); role == "user" && text != "" {
			out.Label = compactLabel(text)
			break
		}
	}
	if out.StartedAt.IsZero() {
		base := filepath.Base(path)
		if len(base) >= len("rollout-2006-01-02T15-04-05") {
			out.StartedAt, _ = time.Parse("2006-01-02T15-04-05", strings.TrimPrefix(base[:len("rollout-2006-01-02T15-04-05")], "rollout-"))
		}
	}
	if out.Label == "" {
		out.Label = "Codex session"
	}
	if projectRoot != "" && filepath.IsAbs(out.WorkingDirectory) && filepath.IsAbs(projectRoot) {
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

// List returns recent sessions only. It never chooses one implicitly.
func (s Source) List(projectRoot string) ([]sessionimport.Summary, error) {
	if err := s.validateRoot(); err != nil {
		return nil, err
	}
	var out []sessionimport.Summary
	remaining := lowerBudget(s.listBudget, listEntryLimit)
	inspections := lowerBudget(s.inspectionBudget, listInspectionLimit)
	candidates := lowerBudget(s.candidateBudget, listCandidateLimit)
	seen := map[string]bool{}
	for i := 0; i < listDays && remaining > 0 && inspections > 0; i++ {
		day := s.today().AddDate(0, 0, -i)
		path := bucket(s.Root, day)
		found, _, err := names(path, remaining)
		if err != nil {
			return nil, err
		}
		remaining -= len(found)
		// Filename order is timestamp order within a date bucket.
		for j := len(found) - 1; j >= 0 && inspections > 0; j-- {
			id := fileID(found[j])
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			inspections--
			if sum, ok := summary(filepath.Join(path, found[j]), id, projectRoot); ok {
				out = append(out, sum)
			}
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
	if len(out) > candidates {
		out = out[:candidates]
	}
	return out, nil
}

// Resolve searches known year/month/day buckets for an exact ID. It never
// opens another session file, and an exhausted work budget is distinct from
// a complete no-match search.
func (s Source) Resolve(id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("invalid Codex session ID %q", id)
	}
	if err := s.validateRoot(); err != nil {
		return "", err
	}
	remaining := lowerBudget(s.lookupBudget, lookupEntryLimit)
	var walk func(string, int) (string, error)
	walk = func(path string, depth int) (string, error) {
		if remaining <= 0 {
			return "", sessionimport.ErrLookupIncomplete
		}
		found, more, err := names(path, remaining)
		if err != nil {
			return "", err
		}
		remaining -= len(found)
		for _, name := range found {
			full := filepath.Join(path, name)
			if depth < 3 {
				if !validBucketName(name, depth) || !directory(full) {
					continue
				}
				match, err := walk(full, depth+1)
				if match != "" || err != nil {
					return match, err
				}
			} else if strings.EqualFold(fileID(name), id) && regular(full) {
				return full, nil
			}
		}
		if more {
			return "", sessionimport.ErrLookupIncomplete
		}
		return "", nil
	}
	path, err := walk(s.Root, 0)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", sessionimport.ErrNotFound
	}
	return path, nil
}

func validBucketName(name string, depth int) bool {
	if (depth == 0 && len(name) != 4) || (depth > 0 && len(name) != 2) {
		return false
	}
	for _, c := range name {
		if c < '0' || c > '9' {
			return false
		}
	}
	if depth == 1 {
		return name >= "01" && name <= "12"
	}
	if depth == 2 {
		return name >= "01" && name <= "31"
	}
	return true
}
