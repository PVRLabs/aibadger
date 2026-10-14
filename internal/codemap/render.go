package codemap

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxHTMLBytes = 20 * 1024 * 1024
const dataMarker = "__BADGER_MAP_DATA__"

//go:embed page.html
var pageTemplate string

// htmlWriter bounds every write, including JSON escaping and template assets.
// Metadata is serialized incrementally; no recursive JSON buffer is allocated.
type htmlWriter struct {
	bytes.Buffer
	limit int
	err   error
}

func (w *htmlWriter) add(s string) {
	if w.err != nil {
		return
	}
	if len(s) > w.limit-w.Len() {
		w.err = fmt.Errorf("HTML size limit (%d bytes) exceeded", w.limit)
		return
	}
	w.Buffer.WriteString(s)
}

func (w *htmlWriter) value(v any) {
	if w.err != nil {
		return
	}
	if s, ok := v.(string); ok {
		w.add(`"`)
		for len(s) > 0 && w.err == nil {
			n := min(len(s), 1024)
			// Never split a UTF-8 sequence between independently escaped chunks.
			if n < len(s) {
				for start := n - 1; start >= max(0, n-utf8.UTFMax+1); start-- {
					if utf8.RuneStart(s[start]) {
						_, size := utf8.DecodeRuneInString(s[start:])
						if start+size > n {
							n = start
						}
						break
					}
				}
			}
			data, err := json.Marshal(s[:n])
			if err != nil {
				w.err = err
				return
			}
			w.add(string(data[1 : len(data)-1]))
			s = s[n:]
		}
		w.add(`"`)
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		w.err = err
		return
	}
	w.add(string(data))
}

func (w *htmlWriter) languages(stats map[string]languageStats) {
	if w.err != nil {
		return
	}
	names := make([]string, 0, len(stats))
	for name := range stats {
		names = append(names, name)
	}
	sort.Strings(names)
	w.add("{")
	for i, name := range names {
		if w.err != nil {
			return
		}
		if i > 0 {
			w.add(",")
		}
		w.value(name)
		w.add(":")
		w.value(stats[name])
	}
	w.add("}")
}

func (w *htmlWriter) directory(node *directory, depth int) {
	if w.err != nil {
		return
	}
	if depth > maxDirectoryDepth {
		w.err = fmt.Errorf("directory depth limit (%d) exceeded while rendering", maxDirectoryDepth)
		return
	}
	if node == nil {
		w.err = fmt.Errorf("cannot render missing directory metadata")
		return
	}
	w.add(`{"name":`)
	w.value(node.Name)
	w.add(`,"path":`)
	w.value(node.Path)
	w.add(`,"lines":`)
	w.value(node.Lines)
	w.add(`,"files":`)
	w.value(node.Files)
	w.add(`,"directLines":`)
	w.value(node.DirectLines)
	w.add(`,"directFiles":`)
	w.value(node.DirectFiles)
	w.add(`,"languages":`)
	w.languages(node.Languages)
	w.add(`,"directLanguages":`)
	w.languages(node.DirectByLang)
	w.add(`,"fileList":[`)
	for i, file := range node.FileList {
		if w.err != nil {
			return
		}
		if i > 0 {
			w.add(",")
		}
		w.add(`{"name":`)
		w.value(file.Name)
		w.add(`,"lines":`)
		w.value(file.Lines)
		w.add("}")
	}
	w.add(`],"children":[`)
	for i, child := range node.Children {
		if w.err != nil {
			return
		}
		if i > 0 {
			w.add(",")
		}
		w.directory(child, depth+1)
	}
	w.add("]}")
}

func renderHTML(root *directory) ([]byte, error) {
	return renderHTMLTemplate(root, pageTemplate, maxHTMLBytes)
}

func renderHTMLTemplate(root *directory, page string, limit int) ([]byte, error) {
	marker := strings.Index(page, dataMarker)
	if marker < 0 {
		return nil, fmt.Errorf("HTML template is missing its data marker")
	}
	output := &htmlWriter{limit: limit}
	output.add(page[:marker])
	output.directory(root, 0)
	output.add(page[marker+len(dataMarker):])
	if output.err != nil {
		return nil, output.err
	}
	return output.Bytes(), nil
}
