package protocol

// This file owns prompt formatting for project topology and extracted code
// context, including truncation metadata and prompt instruction text.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/filegroups"
	"github.com/PVRLabs/aibadger/internal/model"
)

// Formatter handles Schema A and B generation.
type Formatter struct {
	MaxPackages          int // 0 means no limit
	MaxContextFileBytes  int // 0 means no limit
	MaxTotalContextBytes int // 0 means no limit
	Focus                Focus
	Instructions         PromptInstructions
	customInstructions   bool
}

// NewFormatter creates a new Formatter instance.
func NewFormatter() *Formatter {
	return &Formatter{
		MaxContextFileBytes:  defaults.MaxContextFileBytes,
		MaxTotalContextBytes: defaults.MaxTotalContextBytes,
		Focus:                FocusCode,
		Instructions:         DefaultInstructions,
	}
}

// SetFocus updates the prompt framing preset while preserving custom
// instructions when the caller has already overridden the prompt text.
func (f *Formatter) SetFocus(focus Focus) {
	if f == nil {
		return
	}
	f.Focus = NormalizeFocus(focus)
	if f.customInstructions {
		return
	}
	f.Instructions = InstructionsForFocus(f.Focus)
}

// SetPromptInstructions overrides the prompt text and marks it as custom so
// future focus changes do not clobber the caller-provided contract.
func (f *Formatter) SetPromptInstructions(instr PromptInstructions) {
	if f == nil {
		return
	}
	f.Instructions = instr
	f.customInstructions = true
}

// TaggedFile represents a user-selected file with its resolution metadata.
type TaggedFile struct {
	Path    string
	IsLocal bool
}

// GenerateSchemaA builds Prompt 1: Topology for the LLM.
func (f *Formatter) GenerateSchemaA(t *model.ProjectTopology, query string) string {
	return f.GenerateSchemaAWithTaggedFiles(t, query, nil)
}

// GenerateSchemaAWithTaggedFiles builds Prompt 1: Topology and appends a
// user-selected tagged-files section when paths are supplied.
func (f *Formatter) GenerateSchemaAWithTaggedFiles(t *model.ProjectTopology, query string, taggedFiles []TaggedFile) string {
	var header strings.Builder
	f.writeTopologySection(&header, t, 0)
	header.WriteString("\n[SOURCE TREE]\n")
	headerStr := header.String()

	var packages []string
	for _, m := range t.Modules {
		for _, pkg := range schemaAPackages(m) {
			line := formatPackageLine(pkg)
			if line == "" {
				continue
			}
			packages = append(packages, line)
		}
	}

	var etcBuilder strings.Builder
	f.writeExternalContextSection(&etcBuilder, t)
	f.writeTaggedFilesSection(&etcBuilder, taggedFiles)
	etcBuilder.WriteString("\n")
	etcStr := etcBuilder.String()

	instr := f.currentInstructions()
	footer := fmt.Sprintf(instr.SchemaAConstraint, query)

	keep := len(packages)

	if f.MaxTotalContextBytes > 0 && keep > 0 {
		fixedSize := len(headerStr) + len(etcStr) + len(footer)
		available := f.MaxTotalContextBytes - fixedSize
		if available < 0 {
			available = 0
		}
		bodySize := 0
		for _, line := range packages {
			bodySize += len(line) + 1
		}
		for keep > 0 && bodySize > available {
			keep--
			bodySize -= len(packages[keep]) + 1
		}
	}

	if f.MaxPackages > 0 && keep > f.MaxPackages {
		keep = f.MaxPackages
	}

	var result strings.Builder
	result.WriteString(headerStr)
	for i := 0; i < keep; i++ {
		result.WriteString(packages[i])
		result.WriteString("\n")
	}
	if keep < len(packages) {
		result.WriteString("... [Truncated due to size limit] ...\n")
	}
	result.WriteString(etcStr)
	result.WriteString(footer)

	return result.String()
}

func (f *Formatter) writeExternalContextSection(sb *strings.Builder, t *model.ProjectTopology) {
	if t == nil || len(t.ExternalContext) == 0 {
		return
	}
	sb.WriteString("\n[EXTERNAL CONTEXT]\n")
	for _, ctx := range t.ExternalContext {
		sb.WriteString(fmt.Sprintf("%s [read-only]\n", ctx.Path))
		if len(ctx.Top) == 0 {
			sb.WriteString("Top: none\n")
			continue
		}
		parts := make([]string, 0, len(ctx.Top))
		for _, item := range ctx.Top {
			name := item.Name
			if item.IsDir {
				name += "/"
			}
			parts = append(parts, name)
		}
		sb.WriteString(fmt.Sprintf("Top: %s\n", strings.Join(parts, ", ")))
	}
}

func (f *Formatter) writeTaggedFilesSection(sb *strings.Builder, taggedFiles []TaggedFile) {
	if len(taggedFiles) == 0 {
		return
	}
	sb.WriteString("\n[USER TAGGED FILES]\n")
	for _, tf := range taggedFiles {
		sb.WriteString(fmt.Sprintf("FILE:%s\n", tf.Path))
	}
}

func (f *Formatter) writeTopologySection(sb *strings.Builder, t *model.ProjectTopology, activeExtractions int) {
	sb.WriteString("[PROJECT TOPOLOGY]\n")
	if len(t.Languages) > 0 {
		sb.WriteString(fmt.Sprintf("Languages: %s\n", strings.Join(t.Languages, ", ")))
	}
	if shouldPrintPrimary(t) {
		sb.WriteString(fmt.Sprintf("Primary: %s\n", t.PrimaryLanguage))
	}
	if len(t.Stack) > 0 {
		sb.WriteString(fmt.Sprintf("Stack: %s\n", strings.Join(t.Stack, ", ")))
	}
	if t.Structure != "" && t.Structure != "Unknown" {
		sb.WriteString(fmt.Sprintf("Structure: %s\n", t.Structure))
	}
	if activeExtractions > 0 {
		sb.WriteString(fmt.Sprintf("Active Extractions: %d files\n", activeExtractions))
	}
}

func schemaAPackages(module model.Module) []model.Package {
	var packages []model.Package
	packageIndexes := make(map[string]int)

	for _, sourceRoot := range module.SourceRoots {
		for _, pkg := range sourceRoot.Packages {
			idx, exists := packageIndexes[pkg.Path]
			if !exists {
				packageIndexes[pkg.Path] = len(packages)
				packages = append(packages, model.Package{
					Name:      pkg.Name,
					Path:      pkg.Path,
					FileCount: pkg.FileCount,
					Heaviest:  pkg.Heaviest,
					TopFiles:  append([]model.FileSummary(nil), pkg.TopFiles...),
					AuxFiles:  append([]model.FileSummary(nil), pkg.AuxFiles...),
				})
				continue
			}
			packages[idx] = mergeSchemaAPackage(packages[idx], pkg)
		}
	}

	return packages
}

func mergeSchemaAPackage(left, right model.Package) model.Package {
	left.FileCount += right.FileCount
	for _, file := range right.TopFiles {
		left.TopFiles = addSchemaATopFile(left.TopFiles, file)
	}
	for _, file := range right.AuxFiles {
		left.AuxFiles = addSchemaAAuxFile(left.AuxFiles, file)
	}
	if len(left.TopFiles) > 0 {
		left.Heaviest = model.HeaviestFile{
			Name: left.TopFiles[0].Name,
			Path: left.TopFiles[0].Path,
			Size: left.TopFiles[0].Size,
			Kind: left.TopFiles[0].Kind,
		}
	}
	return left
}

func addSchemaATopFile(files []model.FileSummary, file model.FileSummary) []model.FileSummary {
	return addSchemaFile(files, file, sortByPriorityThenSizeDesc)
}

func addSchemaAAuxFile(files []model.FileSummary, file model.FileSummary) []model.FileSummary {
	return addSchemaFile(files, file, sortByKindThenSizeDesc)
}

// addSchemaFile adds file if not duplicate (by path), then sorts using the provided ordering function.
func addSchemaFile(files []model.FileSummary, file model.FileSummary, sortFn func([]model.FileSummary)) []model.FileSummary {
	for _, existing := range files {
		if existing.Path == file.Path {
			return files
		}
	}
	files = append(files, file)
	sortFn(files)
	return files
}

func sortByPriorityThenSizeDesc(files []model.FileSummary) {
	sort.SliceStable(files, func(i, j int) bool {
		pi := schemaATopFilePriority(files[i])
		pj := schemaATopFilePriority(files[j])
		if pi != pj {
			return pi > pj
		}
		if files[i].Size == files[j].Size {
			return files[i].Path < files[j].Path
		}
		return files[i].Size > files[j].Size
	})
}

func sortByKindThenSizeDesc(files []model.FileSummary) {
	sort.SliceStable(files, func(i, j int) bool {
		if schemaAAuxFileKindRank(files[i].Kind) != schemaAAuxFileKindRank(files[j].Kind) {
			return schemaAAuxFileKindRank(files[i].Kind) < schemaAAuxFileKindRank(files[j].Kind)
		}
		if files[i].Size == files[j].Size {
			return files[i].Path < files[j].Path
		}
		return files[i].Size > files[j].Size
	})
}

func schemaAAuxFileKindRank(kind string) int {
	switch kind {
	case model.FileKindBinary:
		return 0
	case model.FileKindAsset:
		return 1
	default:
		return 2
	}
}

func schemaATopFilePriority(file model.FileSummary) int {
	lowerPath := strings.ToLower(file.Path)
	base := strings.ToLower(filepath.Base(file.Path))
	ext := strings.ToLower(filepath.Ext(base))

	if isSchemaACriticalGuidanceDoc(base) {
		return 100
	}
	if isSchemaAIdentityManifest(base) {
		return 98
	}
	if isSchemaAOperationalConfigFile(base) {
		return 95
	}
	if priority := schemaAHighSignalDocPriority(lowerPath, base); priority > 0 {
		return priority
	}
	if isSchemaARootStaticSiteEntryPath(lowerPath, base) {
		return 82
	}
	if isRootWebResourceName(base) {
		return 80
	}
	if isKnownStaticWebPath(lowerPath) {
		return 80
	}
	if ext == ".sh" || ext == ".bash" || ext == ".zsh" {
		return 70
	}
	if file.Kind == model.FileKindAsset {
		return 35
	}
	if file.Kind == model.FileKindBinary {
		return 30
	}
	return 60
}

func isSchemaACriticalGuidanceDoc(base string) bool {
	return filegroups.IsCriticalGuidanceDoc(base)
}

func isSchemaAIdentityManifest(base string) bool {
	return filegroups.IsIdentityManifest(base)
}

func isSchemaAOperationalConfigFile(base string) bool {
	return filegroups.IsOperationalConfigFile(base)
}

func isSchemaARootStaticSiteEntryPath(lowerPath, base string) bool {
	return filegroups.IsRootStaticSiteEntryPath(lowerPath, base)
}

func schemaAHighSignalDocPriority(lowerPath, base string) int {
	if filepath.Ext(base) != ".md" {
		return 0
	}
	if strings.Contains(lowerPath, "archive"+string(filepath.Separator)) ||
		strings.HasSuffix(lowerPath, "archive") {
		return 0
	}
	if !isShallowDocumentationPath(lowerPath) {
		return 0
	}
	if isArchitectureLikeDoc(base) {
		return 92
	}
	if isPlanningArtifactDoc(base) {
		return 85
	}
	return 90
}

func formatPackageLine(pkg model.Package) string {
	top := formatTopFiles(pkg.Path, pkg.TopFiles, pkg.Heaviest)
	hasTop := top != "none"
	hasAux := len(pkg.AuxFiles) > 0
	if !hasTop && !hasAux {
		return ""
	}

	line := fmt.Sprintf("Pkg: %s [%d files] -> ", displayPackagePath(pkg.Path), pkg.FileCount)
	if hasTop {
		line += fmt.Sprintf("Top: %s", top)
	}
	if hasAux {
		if hasTop {
			line += "; "
		}
		line += fmt.Sprintf("Aux: %s", formatFileList(pkg.Path, pkg.AuxFiles))
	}
	return line
}

// GenerateSchemaB builds the context prompt with extracted code.
func (f *Formatter) GenerateSchemaB(t *model.ProjectTopology, extractions []ExtractionResult, query string) (string, []ExtractionMetadata) {
	var metadata []ExtractionMetadata
	processed := make([]ExtractionResult, 0, len(extractions))

	// 1. Per-file trimming
	for _, e := range extractions {
		meta := ExtractionMetadata{
			Path:         e.Path,
			OriginalSize: len(e.Content),
		}

		content := e.Content
		if f.MaxContextFileBytes > 0 && len(content) > f.MaxContextFileBytes {
			content = f.trimContent(content, f.MaxContextFileBytes)
			meta.Truncated = true
		}

		processed = append(processed, ExtractionResult{
			Path:     e.Path,
			Content:  content,
			FullFile: e.FullFile,
		})
		metadata = append(metadata, meta)
	}

	// 2. Compute task and output constraints once, outside the drop loop
	instr := f.currentInstructions()
	constraint := fmt.Sprintf(instr.SchemaBConstraint, query)

	// 3. Total truncation (Drop Last File)
	if f.MaxTotalContextBytes > 0 {
		for {
			body := f.buildSchemaBBody(t, processed, metadata, constraint)
			if len(body) <= f.MaxTotalContextBytes || len(processed) == 0 {
				break
			}
			lastIdx := len(processed) - 1
			metadata[lastIdx].Dropped = true
			processed = processed[:lastIdx]
		}
	}

	body := f.buildSchemaBBody(t, processed, metadata, constraint)
	return body, metadata
}

func (f *Formatter) buildSchemaBBody(t *model.ProjectTopology, extractions []ExtractionResult, metadata []ExtractionMetadata, constraint string) string {
	var sb strings.Builder
	f.writeTopologySection(&sb, t, len(extractions))

	sb.WriteString(constraint)
	sb.WriteString("\n[CONTEXT]\n")
	for i, e := range extractions {
		label := "Extracted Span"
		if strings.HasPrefix(e.Content, "Binary file: ") {
			label = "Binary Summary"
		} else if e.FullFile {
			label = "Full File"
		}
		if i < len(metadata) && metadata[i].Truncated {
			label += ", Truncated"
		}
		sb.WriteString(fmt.Sprintf("--- File: %s (%s) ---\n", e.Path, label))
		sb.WriteString(e.Content)
		sb.WriteString("\n--- End File ---\n")
	}

	return sb.String()
}

func shouldPrintPrimary(t *model.ProjectTopology) bool {
	return len(t.Languages) > 1 && t.PrimaryLanguage != "" && t.PrimaryLanguage != "Unknown"
}

func (f *Formatter) trimContent(content string, limit int) string {
	if limit <= 0 || len(content) <= limit {
		return content
	}

	half := limit / 2
	start := content[:half]
	// Use DecodeLastRuneInString to find safe boundary backward
	for len(start) > 0 {
		r, size := utf8.DecodeLastRuneInString(start)
		if r != utf8.RuneError || size > 1 {
			break
		}
		start = start[:len(start)-1]
	}

	end := content[len(content)-half:]
	// Use DecodeRuneInString to find safe boundary forward
	for len(end) > 0 {
		r, size := utf8.DecodeRuneInString(end)
		if r != utf8.RuneError || size > 1 {
			break
		}
		end = end[size:]
	}

	truncatedBytes := len(content) - len(start) - len(end)
	return fmt.Sprintf("%s\n... [Truncated %d bytes] ...\n%s", start, truncatedBytes, end)
}

func (f *Formatter) currentInstructions() PromptInstructions {
	if f == nil {
		return DefaultInstructions
	}
	expected := InstructionsForFocus(f.Focus)
	if f.customInstructions || f.Instructions != expected {
		if f.Instructions != (PromptInstructions{}) {
			return f.Instructions
		}
	}
	if expected == (PromptInstructions{}) {
		return f.Instructions
	}
	return expected
}

// ExtractionResult stores the result of a single extraction command.
type ExtractionResult struct {
	Path     string
	Content  string
	FullFile bool
}

// ExtractionMetadata tracks trimming status for the TUI and sidecars.
type ExtractionMetadata struct {
	Path         string
	OriginalSize int
	Truncated    bool // Per-file truncation happened
	Dropped      bool // File was dropped entirely due to total limit
}

func formatTopFiles(packagePath string, files []model.FileSummary, heaviest model.HeaviestFile) string {
	if len(files) == 0 && heaviest.Name != "" {
		files = []model.FileSummary{{
			Name: heaviest.Name,
			Path: heaviest.Path,
			Size: heaviest.Size,
		}}
	}
	if len(files) == 0 {
		return "none"
	}

	return formatFileList(packagePath, files)
}

func formatFileList(packagePath string, files []model.FileSummary) string {
	parts := make([]string, 0, len(files))
	for _, file := range files {
		name := displayPathRelativeToPackage(packagePath, file)
		parts = append(parts, formatFileSummary(name, file))
	}
	return strings.Join(parts, ", ")
}

func formatFileSummary(name string, file model.FileSummary) string {
	if file.Kind == model.FileKindAsset || file.Kind == model.FileKindBinary {
		return fmt.Sprintf("%s (%s, %s)", name, FormatFileSize(file.Size), file.Kind)
	}
	return fmt.Sprintf("%s (%s)", name, FormatFileSize(file.Size))
}

func FormatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%dB", size)
	}
	return fmt.Sprintf("%dKB", size/1024)
}

func displayPackagePath(path string) string {
	if path == "" {
		return "."
	}
	return path
}

func displayPathRelativeToPackage(packagePath string, file model.FileSummary) string {
	if file.Path == "" {
		return file.Name
	}
	if packagePath == "" {
		return file.Path
	}

	rel, err := filepath.Rel(packagePath, file.Path)
	if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		return rel
	}
	if file.Name != "" {
		return file.Name
	}
	return file.Path
}
