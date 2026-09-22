package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/sessionimport"
)

// Read at most 1 MiB: 128 KiB head and 896 KiB tail. A single oversized
// final record may hide conversation immediately before it; this MVP accepts
// that limit and marks the source as truncated. Ranges are parsed independently.
const (
	maxSourceRead = 1024 * 1024
	headRead      = 128 * 1024
)

type message struct{ role, text, id string }

func conversationRow(row record) (string, string, string) {
	if row.Type == "response_item" {
		var p struct {
			Type    string `json:"type"`
			ID      string `json:"id"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		// A user-role response item may contain environment, abort, or other
		// contextual input. Only user-message events establish user turns.
		if json.Unmarshal(row.Payload, &p) != nil || p.Type != "message" || p.Role != "assistant" {
			return "", "", ""
		}
		var parts []string
		for _, c := range p.Content {
			if c.Type == "output_text" {
				parts = append(parts, c.Text)
			}
		}
		return p.Role, normalize(strings.Join(parts, "\n")), p.ID
	}
	if row.Type == "event_msg" {
		var p struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Item    struct {
				Type    string `json:"type"`
				ID      string `json:"id"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"item"`
		}
		if json.Unmarshal(row.Payload, &p) != nil {
			return "", "", ""
		}
		switch p.Type {
		case "user_message":
			return "user", normalize(p.Message), ""
		case "agent_message":
			return "assistant", normalize(p.Message), ""
		case "item_completed":
		default:
			return "", "", ""
		}
		role, kind := "", ""
		switch p.Item.Type {
		case "UserMessage":
			role, kind = "user", "text"
		case "AgentMessage":
			role, kind = "assistant", "Text"
		default:
			return "", "", ""
		}
		var parts []string
		for _, c := range p.Item.Content {
			if c.Type == kind {
				parts = append(parts, c.Text)
			}
		}
		return role, normalize(strings.Join(parts, "\n")), p.Item.ID
	}
	return "", "", ""
}

func normalize(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	return s
}

func parseRange(data []byte, skipFirst, skipLast bool) []message {
	if skipFirst {
		if at := bytes.IndexByte(data, '\n'); at >= 0 {
			data = data[at+1:]
		} else {
			return nil
		}
	}
	if skipLast {
		if at := bytes.LastIndexByte(data, '\n'); at >= 0 {
			data = data[:at+1]
		} else {
			return nil
		}
	}
	var out []message
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var row record
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		role, content, id := conversationRow(row)
		if role != "" && content != "" {
			out = append(out, message{role, content, id})
		}
	}
	return out
}

func boundedRead(f *os.File, size int64) ([]message, int64, bool, error) {
	if size <= maxSourceRead {
		data, err := io.ReadAll(io.LimitReader(f, maxSourceRead))
		if err != nil {
			return nil, 0, false, err
		}
		// A file may grow during the read; mark source clipping if it did.
		current, err := f.Stat()
		clipped := err == nil && current.Size() > int64(len(data))
		return parseRange(data, false, clipped), int64(len(data)), clipped, nil
	}
	head := make([]byte, headRead)
	hn, err := f.ReadAt(head, 0)
	if err != nil && err != io.EOF {
		return nil, 0, false, err
	}
	tail := make([]byte, maxSourceRead-headRead)
	tailStart := size - int64(len(tail))
	tn, err := f.ReadAt(tail, tailStart)
	if err != nil && err != io.EOF {
		return nil, 0, false, err
	}
	first := parseRange(head[:hn], false, true)
	// The tail reaches the observed EOF unless the file grew while reading.
	current, statErr := f.Stat()
	grew := statErr == nil && current.Size() > size
	last := parseRange(tail[:tn], true, grew)
	return append(first, last...), int64(hn + tn), true, nil
}

// Extract resolves an opaque ID again at read time. No path supplied by
// metadata or callers is opened. A disappearing session returns a clear error.
func (s Source) Extract(id string, limits sessionimport.Limits) (sessionimport.Conversation, error) {
	path, err := s.Resolve(id)
	if err != nil {
		return sessionimport.Conversation{}, err
	}
	if !regular(path) {
		return sessionimport.Conversation{}, sessionimport.ErrNotFound
	}
	f, err := os.Open(path)
	if err != nil {
		return sessionimport.Conversation{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return sessionimport.Conversation{}, err
	}
	if !stat.Mode().IsRegular() {
		return sessionimport.Conversation{}, sessionimport.ErrNotFound
	}
	rows, _, sourceClipped, err := boundedRead(f, stat.Size())
	if err != nil {
		return sessionimport.Conversation{}, err
	}
	// Shared item IDs are the only reliable duplicate relationship observed
	// between event_msg and response_item. Equal text with different IDs stays.
	seen := map[string]bool{}
	var ordered []message
	for _, row := range rows {
		if row.id != "" && seen[row.id] {
			continue
		}
		if row.id != "" {
			seen[row.id] = true
		}
		ordered = append(ordered, row)
	}
	if len(ordered) == 0 {
		return sessionimport.Conversation{}, sessionimport.ErrNoConversation
	}
	capBytes := limits.MaxOutputBytes
	if capBytes <= 0 || capBytes > sessionimport.DefaultMaxOutputBytes {
		capBytes = sessionimport.DefaultMaxOutputBytes
	}
	header := fmt.Sprintf("Codex session %s\n", id)
	// Reserve enough room for the size/truncation line before selecting text.
	const markerReserve = 160
	available := capBytes - len(header) - markerReserve
	if available < 64 {
		return sessionimport.Conversation{}, fmt.Errorf("import output limit too small")
	}
	var selected []string
	used := 0
	outputClipped := false
	for i := len(ordered) - 1; i >= 0; i-- {
		piece := fmt.Sprintf("\n%s: %s\n", ordered[i].role, ordered[i].text)
		if len(piece) > available-used {
			outputClipped = true
			if len(selected) == 0 {
				prefix := "\n" + ordered[i].role + ": [earlier text omitted] "
				piece = prefix + suffixUTF8(ordered[i].text, available-used-len(prefix)-1) + "\n"
				selected = append(selected, piece)
			}
			break
		}
		selected = append(selected, piece)
		used += len(piece)
	}
	if len(selected) == 0 {
		return sessionimport.Conversation{}, sessionimport.ErrNoConversation
	}
	var body strings.Builder
	for i := len(selected) - 1; i >= 0; i-- {
		body.WriteString(selected[i])
	}
	truncated := sourceClipped || outputClipped
	marker := ""
	if truncated {
		// Imported byte count describes the final attachment including metadata.
		for n := 0; n < 3; n++ {
			count := len(header) + len(body.String()) + len(marker)
			marker = fmt.Sprintf("[source bytes≈%d; imported bytes=%d; older context truncated]\n", stat.Size(), count)
		}
	}
	text := header + marker + body.String()
	if len(text) > capBytes {
		return sessionimport.Conversation{}, fmt.Errorf("import output exceeded limit")
	}
	return sessionimport.Conversation{Text: text, SourceBytes: stat.Size(), ImportedBytes: len(text), Truncated: truncated}, nil
}

func suffixUTF8(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	s = s[len(s)-max:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}
