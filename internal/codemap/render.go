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
const reproductionMarker = "__BADGER_MAP_REPRO__"
const reproductionPanelMarker = "__BADGER_MAP_REPRO_PANEL__"
const reproductionControlHTML = `<button id="regeneration" class="control regeneration-toggle" type="button" aria-expanded="false" aria-controls="regeneration-panel"><span id="regeneration-arrow" aria-hidden="true">▸</span><span class="exclusion-notice" aria-hidden="true">!</span><span>Exclusions applied</span></button>`
const reproductionHTML = `<section id="regeneration-panel" class="regeneration-panel" aria-label="Regeneration instructions" hidden>
<p>Regenerate from the mapped directory (the repository root when mapping the whole repository).
Use the same source tree, Badger version and settings. Version is provenance, not a pinned executable.
Output is written in the current directory; only the destination report is automatically omitted.
Remove or explicitly exclude stale reports for equivalent counts, including reports in nested directories.</p>
<div class="folder-heading"><label for="regeneration-command">Regeneration command · <span id="regeneration-shell"></span></label>
<button id="regeneration-copy" class="control copy-path" type="button" title="Copy regeneration command" aria-label="Copy regeneration command"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/></svg></button></div>
<textarea id="regeneration-command" class="copy-fallback" readonly></textarea>
<span id="regeneration-status" role="status" aria-live="polite"></span>
</section>`

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
	w.add("{")
	if depth == 0 {
		if node.GeneratorVersion != "" {
			w.add(`"generatorVersion":`)
			w.value(node.GeneratorVersion)
			w.add(",")
		}
		if settings := node.Reproduction; settings != nil {
			w.add(`"reproduction":{"patterns":[`)
			for i, pattern := range settings.Patterns {
				if w.err != nil {
					return
				}
				if i > 0 {
					w.add(",")
				}
				w.value(pattern)
			}
			w.add(`],"shell":`)
			w.value(settings.Shell)
			w.add(`,"command":`)
			w.value(settings.Command)
			w.add("},")
		}
	}
	w.add(`"name":`)
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
	section, control := "", ""
	if root != nil && root.Reproduction != nil {
		section, control = reproductionHTML, reproductionControlHTML
	}
	page = strings.Replace(page, reproductionMarker, control, 1)
	page = strings.Replace(page, reproductionPanelMarker, section, 1)

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
