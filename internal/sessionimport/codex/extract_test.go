package codex

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/sessionimport"
)

func event(kind, id, text string) string {
	contentType := "text"
	if kind == "AgentMessage" {
		contentType = "Text"
	}
	return fmt.Sprintf("{\"type\":\"event_msg\",\"payload\":{\"type\":\"item_completed\",\"item\":{\"type\":%q,\"id\":%q,\"content\":[{\"type\":%q,\"text\":%q}]}}}\n", kind, id, contentType, text)
}

func response(role, id, text string) string {
	kind := "input_text"
	if role == "assistant" {
		kind = "output_text"
	}
	return fmt.Sprintf("{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"id\":%q,\"role\":%q,\"content\":[{\"type\":%q,\"text\":%q}]}}\n", id, role, kind, text)
}

func directEvent(kind, text string) string {
	return fmt.Sprintf("{\"type\":\"event_msg\",\"payload\":{\"type\":%q,\"message\":%q}}\n", kind, text)
}

func TestExtractConversationFilteringAndDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	data := event("UserMessage", "same", "Please fix 🦡") + response("user", "same", "Please fix 🦡") +
		event("UserMessage", "different", "Please fix 🦡") + event("AgentMessage", "answer", "Done") +
		response("assistant", "assistant-response", "Also done") +
		response("user", "context", "SECRET ENVIRONMENT CONTEXT") +
		response("user", "abort", "<turn_aborted> SECRET ABORT CONTEXT") +
		`{"type":"response_item","payload":{"type":"custom_tool_call_output","output":"SECRET TOOL BLOB"}}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"HIDDEN"}]}}` + "\n" +
		`{"type":"unknown","payload":{"text":"UNKNOWN"}}` + "\n" +
		"{malformed\n" + `{"type":"response_item","payload":{"type":"message","role":"assistant"` // partial final
	path := fixture(t, root, day, idA, root, data)
	before, _ := os.ReadFile(path)
	s := Source{Root: root}
	got, err := s.Extract(idA, sessionimport.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got.Text, "Please fix 🦡") != 2 || !strings.Contains(got.Text, "assistant: Done") || !strings.Contains(got.Text, "assistant: Also done") {
		t.Fatalf("conversation = %q", got.Text)
	}
	for _, bad := range []string{"SECRET", "HIDDEN", "UNKNOWN", "malformed"} {
		if strings.Contains(got.Text, bad) {
			t.Fatalf("included %s", bad)
		}
	}
	if got.Truncated || got.ImportedBytes != len(got.Text) || !utf8.ValidString(got.Text) {
		t.Fatalf("result = %+v", got)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("source modified")
	}
}

func TestExtractNoConversationAndMissing(t *testing.T) {
	root := t.TempDir()
	s := Source{Root: root}
	fixture(t, root, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), idA, root, `{"type":"world_state"}`+"\n"+response("user", "context", "environment context"))
	if _, err := s.Extract(idA, sessionimport.Limits{}); !errors.Is(err, sessionimport.ErrNoConversation) {
		t.Fatalf("empty extraction = %v", err)
	}
	if _, err := s.Extract(idB, sessionimport.Limits{}); !errors.Is(err, sessionimport.ErrNotFound) {
		t.Fatalf("missing extraction = %v", err)
	}
}

func TestDirectEventsAndContextualUserResponses(t *testing.T) {
	root := t.TempDir()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	data := response("user", "environment", strings.Repeat("SECRET CONTEXT ", 2000)) +
		directEvent("user_message", "real task") + directEvent("agent_message", "real answer")
	fixture(t, root, day, idA, root, data)
	got, err := (Source{Root: root}).Extract(idA, sessionimport.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, "user: real task") || !strings.Contains(got.Text, "assistant: real answer") || strings.Contains(got.Text, "SECRET CONTEXT") {
		t.Fatalf("direct events = %q", got.Text)
	}
	if got.Truncated {
		t.Fatalf("contextual response consumed budget: %+v", got)
	}
}

func TestValidFinalRecordWithoutNewline(t *testing.T) {
	root := t.TempDir()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	last := strings.TrimSuffix(directEvent("agent_message", "newest answer"), "\n")
	fixture(t, root, day, idA, root, directEvent("user_message", "task")+last)
	got, err := (Source{Root: root}).Extract(idA, sessionimport.Limits{})
	if err != nil || !strings.Contains(got.Text, "assistant: newest answer") {
		t.Fatalf("unterminated final record = %+v, %v", got, err)
	}
	fixture(t, root, day, idC, root, directEvent("user_message", "task")+strings.TrimSuffix(response("assistant", "answer", "response answer"), "\n"))
	got, err = (Source{Root: root}).Extract(idC, sessionimport.Limits{})
	if err != nil || !strings.Contains(got.Text, "assistant: response answer") {
		t.Fatalf("unterminated response item = %+v, %v", got, err)
	}
	// The same rule applies to a bounded tail that reaches actual EOF.
	path := fixture(t, root, day, idB, root, directEvent("user_message", "task")+strings.Repeat("{malformed}\n", 100000)+last)
	got, err = (Source{Root: root}).Extract(idB, sessionimport.Limits{})
	if err != nil || !strings.Contains(got.Text, "assistant: newest answer") {
		t.Fatalf("tail final record = %+v, %v", got, err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() <= maxSourceRead {
		t.Fatalf("tail fixture too short: %+v, %v", info, err)
	}
}

func TestExtractBoundsAndTruncationMarker(t *testing.T) {
	root := t.TempDir()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	data := event("UserMessage", "initial", "initial task") + strings.Repeat(`{"type":"response_item","payload":{"type":"custom_tool_call_output","output":"blob"}}`+"\n", 20000) + response("assistant", "latest", strings.Repeat("🦡", 20000))
	path := fixture(t, root, day, idA, root, data)
	got, err := (Source{Root: root}).Extract(idA, sessionimport.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Text) > sessionimport.DefaultMaxOutputBytes || !utf8.ValidString(got.Text) {
		t.Fatalf("bounds = %+v", got)
	}
	if !strings.Contains(got.Text, "source bytes≈") || !strings.Contains(got.Text, "imported bytes=") || !strings.Contains(got.Text, "older context truncated") {
		t.Fatalf("marker missing: %q", got.Text[:min(len(got.Text), 200)])
	}
	if !strings.Contains(got.Text, "🦡") || got.SourceBytes <= maxSourceRead || got.ImportedBytes != len(got.Text) {
		t.Fatalf("sizes = %+v", got)
	}
	stat, _ := os.Stat(path)
	if got.SourceBytes != stat.Size() {
		t.Fatalf("source size = %d, want %d", got.SourceBytes, stat.Size())
	}
}

func TestLargeFinalBlobLimitationIsMarkedTruncated(t *testing.T) {
	root := t.TempDir()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	noise := strings.Repeat(`{"type":"response_item","payload":{"type":"custom_tool_call_output","output":"noise"}}`+"\n", 2500)
	blob := fmt.Sprintf("{\"type\":\"response_item\",\"payload\":{\"type\":\"custom_tool_call_output\",\"output\":%q}}\n", strings.Repeat("BLOB", 800000))
	path := fixture(t, root, day, idA, root, directEvent("user_message", "initial task")+noise+directEvent("agent_message", "recent decision before blob")+blob)
	got, err := (Source{Root: root}).Extract(idA, sessionimport.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || !strings.Contains(got.Text, "older context truncated") || !strings.Contains(got.Text, "initial task") || strings.Contains(got.Text, "recent decision before blob") || strings.Contains(got.Text, "BLOB") {
		t.Fatalf("large final blob limitation: %q", got.Text)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	stat, _ := f.Stat()
	_, read, clipped, err := boundedRead(f, stat.Size())
	if err != nil || !clipped || read > maxSourceRead {
		t.Fatalf("read bound = %d, clipped=%v, err=%v", read, clipped, err)
	}
}

func TestOutputOnlyTruncationAndChangingSource(t *testing.T) {
	root := t.TempDir()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	path := fixture(t, root, day, idA, root, event("UserMessage", "long", strings.Repeat("🦡", 300)))
	got, err := (Source{Root: root}).Extract(idA, sessionimport.Limits{MaxOutputBytes: 300})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Text) > 300 || !utf8.ValidString(got.Text) || !strings.Contains(got.Text, "user: [earlier text omitted]") {
		t.Fatalf("output truncation = %+v", got)
	}
	if got.SourceBytes >= maxSourceRead {
		t.Fatalf("unexpected source clipping: %d", got.SourceBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Truncate(path, int64(len(user("short")))); err != nil {
		t.Fatal(err)
	}
	rows, read, clipped, err := boundedRead(f, maxSourceRead+100)
	if err != nil || read > maxSourceRead || !clipped || len(rows) != 0 {
		t.Fatalf("changing read = %+v, %d, %v, %v", rows, read, clipped, err)
	}
}

func TestIndependentHeadTailBoundaries(t *testing.T) {
	// Construct two ranges with incomplete JSON fragments at both cut points.
	head := []byte(event("UserMessage", "first", "FIRST"))
	head = append(head, []byte(strings.Repeat("X", headRead-len(head)))...)
	tail := []byte(strings.Repeat("Y", 100) + "\n" + response("assistant", "last", "LAST"))
	rows := append(parseRange(head, false, true), parseRange(tail, true, true)...)
	if len(rows) != 2 || rows[0].text != "FIRST" || rows[1].text != "LAST" {
		t.Fatalf("boundary rows = %+v", rows)
	}
	// These fragments would be a valid record if concatenated; independent
	// parsing must still discard both pieces.
	broken := event("UserMessage", "joined", "SHOULD NOT APPEAR")
	cut := len(broken) / 2
	joined := append(parseRange([]byte(event("UserMessage", "a", "A")+broken[:cut]), false, true), parseRange([]byte(broken[cut:]+response("assistant", "b", "B")), true, true)...)
	if len(joined) != 2 || joined[0].text != "A" || joined[1].text != "B" {
		t.Fatalf("joined fragments = %+v", joined)
	}
}
