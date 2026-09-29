package claude

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PVRLabs/aibadger/internal/sessionimport"
)

const testID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
const otherID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

func fixture(t *testing.T, root, project, id, cwd, content string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".jsonl")
	rows := []any{
		map[string]any{"type": "user", "sessionId": id, "timestamp": "2026-09-28T12:00:00Z", "cwd": cwd,
			"message": map[string]any{"role": "user", "content": content}},
		map[string]any{"type": "assistant", "sessionId": id,
			"message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "text", "text": "Useful answer"},
				map[string]any{"type": "tool_use", "name": "Bash"},
			}}},
	}
	var data []byte
	for _, row := range rows {
		line, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		data = append(append(data, line...), '\n')
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveNewestDuplicateSession(t *testing.T) {
	root := t.TempDir()
	old := fixture(t, root, "a-project", testID, t.TempDir(), "Older task")
	newer := fixture(t, root, "z-project", testID, t.TempDir(), "Newer task")
	base := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(old, base, base); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, base.Add(time.Minute), base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	s := Source{Root: root}
	items, err := s.List("")
	if err != nil || len(items) != 1 || items[0].Label != "Newer task" {
		t.Fatalf("discovery=%+v err=%v", items, err)
	}
	path, err := s.Resolve(testID)
	if err != nil || path != newer {
		t.Fatalf("resolve=%q, want %q, err=%v", path, newer, err)
	}
	conversation, err := s.Extract(testID, sessionimport.Limits{})
	if err != nil || !strings.Contains(conversation.Text, "Newer task") {
		t.Fatalf("extract=%+v err=%v", conversation, err)
	}
}

func TestDefaultHonorsConfigDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	s, err := Default()
	if err != nil || s.Root != filepath.Join(root, "projects") {
		t.Fatalf("source=%+v err=%v", s, err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "relative-claude-config")
	s, err = Default()
	if err != nil || !filepath.IsAbs(s.Root) || filepath.Base(s.Root) != "projects" || filepath.Base(filepath.Dir(s.Root)) != "relative-claude-config" {
		t.Fatalf("relative config source=%+v err=%v", s, err)
	}
}

func TestDiscoveryAndExactLookup(t *testing.T) {
	root := t.TempDir()
	project := t.TempDir()
	path := fixture(t, root, "project-one", testID, project, `Fix the "importer"`)
	fixture(t, root, "project-two", otherID, t.TempDir(), "Other task")
	if err := os.WriteFile(filepath.Join(root, "project-one", "bad.jsonl"), []byte("garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	s := Source{Root: root}
	items, err := s.List(project)
	if err != nil || len(items) != 2 || items[0].ID != testID || !items[0].ProjectMatch || items[0].Label != `Fix the "importer"` || items[0].StartedAt.IsZero() {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	got, err := s.Resolve(testID)
	if err != nil || got != path {
		t.Fatalf("resolve=%q err=%v", got, err)
	}
	if _, err := s.Resolve("../bad"); err == nil {
		t.Fatal("accepted invalid ID")
	}
	if _, err := s.Resolve("cccccccc-cccc-cccc-cccc-cccccccccccc"); !errors.Is(err, sessionimport.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestMalformedAndMissingTranscripts(t *testing.T) {
	root := t.TempDir()
	s := Source{Root: root}
	items, err := s.List("")
	if err != nil || len(items) != 0 {
		t.Fatalf("missing root: %+v %v", items, err)
	}
	path := fixture(t, root, "project", testID, t.TempDir(), "Task")
	if err := os.WriteFile(path, []byte("not json\n"+`{"type":"user","sessionId":"`+testID+`","message":{"role":"user","content":[{"type":"tool_result","content":"secret"},{"type":"text","text":"Retained task"}]}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err = s.List("")
	if err != nil || len(items) != 1 || items[0].Label != "Retained task" {
		t.Fatalf("malformed: %+v %v", items, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Extract(testID, sessionimport.Limits{}); !errors.Is(err, sessionimport.ErrNotFound) {
		t.Fatalf("removed: %v", err)
	}
}

func TestExtractionFiltersAndBounds(t *testing.T) {
	root := t.TempDir()
	path := fixture(t, root, "project", testID, t.TempDir(), "Original task")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("invalid\n" + `{"type":"user","isMeta":true,"message":{"role":"user","content":"hidden"}}` + "\n" + `{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"Final response 🦡"}]}}` + "\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	s := Source{Root: root}
	result, err := s.Extract(testID, sessionimport.Limits{})
	if err != nil || !strings.Contains(result.Text, "Original task") || !strings.Contains(result.Text, "Final response 🦡") || strings.Contains(result.Text, "hidden") || strings.Contains(result.Text, "private") || !strings.HasSuffix(result.Text, "\n") {
		t.Fatalf("extract=%+v err=%v", result, err)
	}
	if err := os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"`+strings.Repeat("x", 80*1024)+`"}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = s.Extract(testID, sessionimport.Limits{MaxOutputBytes: 1024})
	if err != nil || !result.Truncated || result.ImportedBytes > 1024 || !strings.Contains(result.Text, "truncated") {
		t.Fatalf("bounded=%+v err=%v", result, err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 1100*1024)+"\n"+`{"type":"user","message":{"role":"user","content":"tail"}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = s.Extract(testID, sessionimport.Limits{})
	if err != nil || !result.Truncated || !strings.Contains(result.Text, "tail") {
		t.Fatalf("tail=%+v err=%v", result, err)
	}
}
