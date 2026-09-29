package claude

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

const (
	maxSourceRead = 1024 * 1024
	headRead      = 128 * 1024
)

type message struct{ role, text string }

func conversationRow(row record) (string, string) {
	if row.IsMeta || row.IsSidechain || (row.Type != "user" && row.Type != "assistant") {
		return "", ""
	}
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(row.Message, &msg) != nil || msg.Role != row.Type {
		return "", ""
	}
	var value string
	if len(msg.Content) > 0 && msg.Content[0] == '"' {
		if json.Unmarshal(msg.Content, &value) != nil {
			return "", ""
		}
	} else {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(msg.Content, &blocks) != nil {
			return "", ""
		}
		var parts []string
		for _, block := range blocks {
			if block.Type == "text" {
				parts = append(parts, block.Text)
			}
		}
		value = strings.Join(parts, "\n")
	}
	return row.Type, normalize(value)
}

func normalize(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}

func parseRange(data []byte, skipFirst, skipLast bool) []message {
	if skipFirst {
		at := bytes.IndexByte(data, '\n')
		if at < 0 {
			return nil
		}
		data = data[at+1:]
	}
	if skipLast {
		at := bytes.LastIndexByte(data, '\n')
		if at < 0 {
			return nil
		}
		data = data[:at+1]
	}
	var out []message
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var row record
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		role, content := conversationRow(row)
		if role != "" && content != "" {
			out = append(out, message{role, content})
		}
	}
	return out
}

func boundedRead(f *os.File, size int64) ([]message, bool, error) {
	if size <= maxSourceRead {
		data, err := io.ReadAll(io.LimitReader(f, maxSourceRead))
		if err != nil {
			return nil, false, err
		}
		current, err := f.Stat()
		clipped := err == nil && current.Size() > int64(len(data))
		return parseRange(data, false, clipped), clipped, nil
	}
	head := make([]byte, headRead)
	hn, err := f.ReadAt(head, 0)
	if err != nil && err != io.EOF {
		return nil, false, err
	}
	tail := make([]byte, maxSourceRead-headRead)
	tn, err := f.ReadAt(tail, size-int64(len(tail)))
	if err != nil && err != io.EOF {
		return nil, false, err
	}
	current, statErr := f.Stat()
	grew := statErr == nil && current.Size() > size
	rows := append(parseRange(head[:hn], false, true), parseRange(tail[:tn], true, grew)...)
	return rows, true, nil
}

func (s Source) Extract(id string, limits sessionimport.Limits) (sessionimport.Conversation, error) {
	path, err := s.Resolve(id)
	if err != nil {
		return sessionimport.Conversation{}, err
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
	rows, sourceClipped, err := boundedRead(f, stat.Size())
	if err != nil {
		return sessionimport.Conversation{}, err
	}
	if len(rows) == 0 {
		return sessionimport.Conversation{}, sessionimport.ErrNoConversation
	}
	capBytes := limits.MaxOutputBytes
	if capBytes <= 0 || capBytes > sessionimport.DefaultMaxOutputBytes {
		capBytes = sessionimport.DefaultMaxOutputBytes
	}
	header := fmt.Sprintf("Claude session %s\n", id)
	const markerReserve = 160
	available := capBytes - len(header) - markerReserve
	if available < 64 {
		return sessionimport.Conversation{}, fmt.Errorf("import output limit too small")
	}
	var selected []string
	used := 0
	outputClipped := false
	for i := len(rows) - 1; i >= 0; i-- {
		piece := fmt.Sprintf("\n%s: %s\n", rows[i].role, rows[i].text)
		if len(piece) > available-used {
			outputClipped = true
			if len(selected) == 0 {
				prefix := "\n" + rows[i].role + ": [earlier text omitted] "
				selected = append(selected, prefix+suffixUTF8(rows[i].text, available-used-len(prefix)-1)+"\n")
			}
			break
		}
		selected = append(selected, piece)
		used += len(piece)
	}
	var body strings.Builder
	for i := len(selected) - 1; i >= 0; i-- {
		body.WriteString(selected[i])
	}
	truncated := sourceClipped || outputClipped
	marker := ""
	if truncated {
		for n := 0; n < 3; n++ {
			count := len(header) + body.Len() + len(marker)
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
