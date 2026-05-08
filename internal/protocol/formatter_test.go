package protocol

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/model"
)

func TestGenerateSchemaA(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"Java"},
		PrimaryLanguage: "Java",
		Stack:           []string{"Maven", "Spring Boot"},
		Structure:       "Single Module",
		Modules: []model.Module{
			{
				Name:     "core",
				Language: "Java",
				SourceRoots: []model.SourceRoot{
					{
						Packages: []model.Package{
							{
								Path:      "src/main/java/com/example/core",
								FileCount: 5,
								Heaviest: model.HeaviestFile{
									Name: "CoreService.java",
									Path: "src/main/java/com/example/core/CoreService.java",
									Size: 10240,
								},
								TopFiles: []model.FileSummary{
									{
										Name: "CoreService.java",
										Path: "src/main/java/com/example/core/CoreService.java",
										Size: 10240,
									},
									{
										Name: "CoreHelper.java",
										Path: "src/main/java/com/example/core/CoreHelper.java",
										Size: 5120,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	query := "Find all services"

	output := formatter.GenerateSchemaA(topology, query)

	if !strings.Contains(output, "[PROJECT TOPOLOGY]") {
		t.Errorf("Expected output to contain [PROJECT TOPOLOGY]")
	}
	if !strings.Contains(output, "Languages: Java") {
		t.Errorf("Expected output to contain project languages")
	}
	if strings.Contains(output, "Primary: Java") {
		t.Errorf("Expected output to omit primary language for single-language project")
	}
	if strings.Contains(output, "Type:") {
		t.Errorf("Expected output to omit project type")
	}
	if !strings.Contains(output, "Stack: Maven, Spring Boot") {
		t.Errorf("Expected output to contain stack information")
	}
	if !strings.Contains(output, "Structure: Single Module") {
		t.Errorf("Expected output to contain project structure")
	}
	if !strings.Contains(output, "Pkg: src/main/java/com/example/core [5 files] -> Top: CoreService.java (10KB), CoreHelper.java (5KB)") {
		t.Errorf("Expected output to contain package and module info")
	}
	if !strings.Contains(output, "[TASK]\nFind all services") {
		t.Errorf("Expected output to contain the task")
	}
	if !strings.Contains(output, "FILE:<path>") {
		t.Errorf("Expected output to contain constraints")
	}
}

func TestGenerateSchemaAIncludesContextSelectionGuidance(t *testing.T) {
	formatter := NewFormatter()
	output := formatter.GenerateSchemaA(&model.ProjectTopology{}, "what should I do next?")

	for _, want := range []string{
		"Target the smallest context set required for the first logical step. Prefer 3-7 entries; exceed 10 only if the immediate step clearly requires broad implementation context.",
		"For planning, explanation, triage, or \"what is this project\" queries, request overview files first",
		"Do not request one file from every package just because the query is broad.",
		"FILE:<path>",
		"PREFIX:<path>#<literal prefix from the start of the target line>",
		"NEAR:<path>#<literal string from a nearby unique line or comment>",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected Prompt 1 to contain %q, got:\n%s", want, output)
		}
	}
}

func TestGenerateSchemaAPreservesMultilineTask(t *testing.T) {
	formatter := NewFormatter()
	task := strings.Join([]string{
		"Review this change for correctness.",
		"",
		"diff --git a/internal/tui/tui.go b/internal/tui/tui.go",
		"@@ -1,3 +1,4 @@",
		" package tui",
		"+new line",
		"-old line",
	}, "\n")

	output := formatter.GenerateSchemaA(&model.ProjectTopology{}, task)

	if !strings.Contains(output, "[TASK]\n"+task+"\n\n[CONSTRAINT]") {
		t.Fatalf("Prompt 1 did not preserve multiline task:\n%s", output)
	}
}

func TestGenerateSchemaAReportsPrimaryOnlyForMultipleLanguages(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"Go", "Java"},
		PrimaryLanguage: "Go",
	}

	output := formatter.GenerateSchemaA(topology, "test")

	if !strings.Contains(output, "Languages: Go, Java\nPrimary: Go") {
		t.Fatalf("expected mixed-language primary header, got:\n%s", output)
	}
}

func TestGenerateSchemaAReportsTopFilesRelativeToPackage(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"Java"},
		PrimaryLanguage: "Java",
		Modules: []model.Module{
			{
				SourceRoots: []model.SourceRoot{
					{
						Packages: []model.Package{
							{
								Path:      "web/src/main/java/pvr/finrecord/repository",
								FileCount: 2,
								TopFiles: []model.FileSummary{
									{
										Name: "OptionTradeRepository.java",
										Path: "web/src/main/java/pvr/finrecord/repository/OptionTradeRepository.java",
										Size: 2048,
									},
									{
										Name: "UserRepository.java",
										Path: "web/src/main/java/pvr/finrecord/repository/UserRepository.java",
										Size: 1024,
									},
								},
							},
							{
								Path:      "web/src/main/java/pvr/finrecord",
								FileCount: 2,
								TopFiles: []model.FileSummary{
									{
										Name: "FinancialRecordsApplication.java",
										Path: "web/src/main/java/pvr/finrecord/FinancialRecordsApplication.java",
										Size: 1024,
									},
									{
										Name: "UserAuthSecurityConfig.java",
										Path: "web/src/main/java/pvr/finrecord/baseapp/security/UserAuthSecurityConfig.java",
										Size: 4096,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	output := formatter.GenerateSchemaA(topology, "test")

	if !strings.Contains(output, "Pkg: web/src/main/java/pvr/finrecord/repository [2 files] -> Top: OptionTradeRepository.java (2KB), UserRepository.java (1KB)") {
		t.Fatalf("expected Java package top files to be package-relative, got:\n%s", output)
	}
	if !strings.Contains(output, "Pkg: web/src/main/java/pvr/finrecord [2 files] -> Top: FinancialRecordsApplication.java (1KB), baseapp/security/UserAuthSecurityConfig.java (4KB)") {
		t.Fatalf("expected bubbled Java top files to preserve child path relative to package, got:\n%s", output)
	}
}

func TestGenerateSchemaAKeepsGoTopFilesCompact(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"Go"},
		PrimaryLanguage: "Go",
		Modules: []model.Module{
			{
				SourceRoots: []model.SourceRoot{
					{
						Packages: []model.Package{
							{
								Path:      "internal/service",
								FileCount: 2,
								TopFiles: []model.FileSummary{
									{
										Name: "order.go",
										Path: "internal/service/order.go",
										Size: 2048,
									},
									{
										Name: "payment.go",
										Path: "internal/service/payment.go",
										Size: 1024,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	output := formatter.GenerateSchemaA(topology, "test")

	if !strings.Contains(output, "Pkg: internal/service [2 files] -> Top: order.go (2KB), payment.go (1KB)") {
		t.Fatalf("expected Go top files to stay compact, got:\n%s", output)
	}
}

func TestGenerateSchemaADisplaysRootPackageAsDot(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"Go"},
		PrimaryLanguage: "Go",
		Modules: []model.Module{
			{
				SourceRoots: []model.SourceRoot{
					{
						Packages: []model.Package{
							{
								Path:      "",
								FileCount: 1,
								TopFiles: []model.FileSummary{
									{
										Name: "main.go",
										Path: "main.go",
										Size: 1024,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	output := formatter.GenerateSchemaA(topology, "test")

	if !strings.Contains(output, "Pkg: . [1 files] -> Top: main.go (1KB)") {
		t.Fatalf("expected root package to render as dot, got:\n%s", output)
	}
	if strings.Contains(output, "Pkg:  [1 files]") {
		t.Fatalf("root package rendered as empty path:\n%s", output)
	}
}

func TestGenerateSchemaACoalescesDuplicatePackageDisplayGroups(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"TypeScript"},
		PrimaryLanguage: "TypeScript",
		Modules: []model.Module{
			{
				SourceRoots: []model.SourceRoot{
					{
						Path: "",
						Role: "Module Overview",
						Packages: []model.Package{
							{
								Path:      "",
								FileCount: 3,
								TopFiles: []model.FileSummary{
									{Name: "package.json", Path: "package.json", Size: 1024, Kind: model.FileKindSource},
									{Name: "tsconfig.json", Path: "tsconfig.json", Size: 512, Kind: model.FileKindSource},
									{Name: "nest-cli.json", Path: "nest-cli.json", Size: 256, Kind: model.FileKindSource},
								},
							},
						},
					},
					{
						Path: "",
						Role: "Documentation",
						Packages: []model.Package{
							{
								Path:      "",
								FileCount: 1,
								TopFiles: []model.FileSummary{
									{Name: "README.md", Path: "README.md", Size: 113, Kind: model.FileKindSource},
								},
							},
						},
					},
				},
			},
		},
	}

	output := formatter.GenerateSchemaA(topology, "explain")

	if got := strings.Count(output, "Pkg: . "); got != 1 {
		t.Fatalf("root package display count = %d, want 1:\n%s", got, output)
	}
	want := "Pkg: . [4 files] -> Top: README.md (113B), package.json (1KB), tsconfig.json (512B), nest-cli.json (256B)"
	if !strings.Contains(output, want) {
		t.Fatalf("expected coalesced root package line, got:\n%s", output)
	}
}

func TestGenerateSchemaARendersAuxBinaryAndAssetFiles(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"JavaScript"},
		PrimaryLanguage: "JavaScript",
		Modules: []model.Module{
			{
				SourceRoots: []model.SourceRoot{
					{
						Packages: []model.Package{
							{
								Path:      "",
								FileCount: 6,
								TopFiles: []model.FileSummary{
									{Name: "index.html", Path: "index.html", Size: 21 * 1024, Kind: model.FileKindSource},
									{Name: "app.js", Path: "app.js", Size: 3 * 1024, Kind: model.FileKindSource},
									{Name: "package.json", Path: "package.json", Size: 1 * 1024, Kind: model.FileKindSource},
								},
								AuxFiles: []model.FileSummary{
									{Name: "android-chrome-512x512.png", Path: "android-chrome-512x512.png", Size: 18 * 1024, Kind: model.FileKindAsset},
									{Name: "badger", Path: "badger", Size: 5519 * 1024, Kind: model.FileKindBinary},
								},
							},
						},
					},
				},
			},
		},
	}

	output := formatter.GenerateSchemaA(topology, "what this project does?")

	want := "Pkg: . [6 files] -> Top: index.html (21KB), app.js (3KB), package.json (1KB); Aux: android-chrome-512x512.png (18KB, asset), badger (5519KB, binary)"
	if !strings.Contains(output, want) {
		t.Fatalf("expected labeled asset and binary aux files, got:\n%s", output)
	}
}

func TestGenerateSchemaAFormatsSubKilobyteFilesAsBytes(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"Go"},
		PrimaryLanguage: "Go",
		Modules: []model.Module{
			{
				SourceRoots: []model.SourceRoot{
					{
						Packages: []model.Package{
							{
								Path:      "",
								FileCount: 4,
								TopFiles: []model.FileSummary{
									{Name: "small.go", Path: "small.go", Size: 512, Kind: model.FileKindSource},
									{Name: "edge.go", Path: "edge.go", Size: 1023, Kind: model.FileKindSource},
									{Name: "onekb.go", Path: "onekb.go", Size: 1024, Kind: model.FileKindSource},
								},
								AuxFiles: []model.FileSummary{
									{Name: "hero.png", Path: "hero.png", Size: 800, Kind: model.FileKindAsset},
								},
							},
						},
					},
				},
			},
		},
	}

	output := formatter.GenerateSchemaA(topology, "test")

	want := "Pkg: . [4 files] -> Top: small.go (512B), edge.go (1023B), onekb.go (1KB); Aux: hero.png (800B, asset)"
	if !strings.Contains(output, want) {
		t.Fatalf("expected byte and KB size formatting, got:\n%s", output)
	}
}

func TestGenerateSchemaAFormatsAuxOnlyPackagesWithoutTopNone(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"Java"},
		PrimaryLanguage: "Java",
		Modules: []model.Module{
			{
				SourceRoots: []model.SourceRoot{
					{
						Packages: []model.Package{
							{
								Path:      filepath.Join("src", "main", "resources", "static", "img"),
								FileCount: 1,
								AuxFiles: []model.FileSummary{
									{
										Name: "logo.png",
										Path: filepath.Join("src", "main", "resources", "static", "img", "logo.png"),
										Size: 10,
										Kind: model.FileKindAsset,
									},
								},
							},
							{
								Path:      "empty",
								FileCount: 0,
							},
						},
					},
				},
			},
		},
	}

	output := formatter.GenerateSchemaA(topology, "test")

	want := "Pkg: src/main/resources/static/img [1 files] -> Aux: logo.png (10B, asset)"
	if !strings.Contains(output, want) {
		t.Fatalf("expected aux-only package without Top none, got:\n%s", output)
	}
	if strings.Contains(output, "Top: none; Aux:") {
		t.Fatalf("aux-only package should not print Top none, got:\n%s", output)
	}
	if strings.Contains(output, "Pkg: empty") {
		t.Fatalf("empty package should not be printed, got:\n%s", output)
	}
}

func TestGenerateSchemaATruncation(t *testing.T) {
	formatter := NewFormatter()
	formatter.MaxPackages = 1
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{
				SourceRoots: []model.SourceRoot{
					{
						Packages: []model.Package{
							{
								Path:      "pkg1",
								FileCount: 1,
								TopFiles:  []model.FileSummary{{Name: "one.go", Path: filepath.Join("pkg1", "one.go"), Size: 1}},
							},
							{
								Path:      "pkg2",
								FileCount: 1,
								TopFiles:  []model.FileSummary{{Name: "two.go", Path: filepath.Join("pkg2", "two.go"), Size: 1}},
							},
						},
					},
				},
			},
		},
	}

	output := formatter.GenerateSchemaA(topology, "test")

	if !strings.Contains(output, "Pkg: pkg1") {
		t.Errorf("Expected pkg1 to be present")
	}
	if strings.Contains(output, "Pkg: pkg2") {
		t.Errorf("Expected pkg2 to be truncated")
	}
	if !strings.Contains(output, "... [Truncated due to size limit] ...") {
		t.Errorf("Expected truncation marker")
	}
}

func TestGenerateSchemaB(t *testing.T) {
	formatter := NewFormatter()
	topology := &model.ProjectTopology{
		Languages:       []string{"Java"},
		PrimaryLanguage: "Java",
	}
	extractions := []ExtractionResult{
		{
			Path:     "src/main/java/com/example/core/CoreService.java",
			Content:  "public class CoreService {}",
			FullFile: true,
		},
	}
	query := "Add a method to CoreService"

	output, _ := formatter.GenerateSchemaB(topology, extractions, query)

	if !strings.Contains(output, "[PROJECT TOPOLOGY]") {
		t.Errorf("Expected output to contain [PROJECT TOPOLOGY]")
	}
	if !strings.Contains(output, "Active Extractions: 1 files") {
		t.Errorf("Expected output to contain active extractions count")
	}
	if !strings.Contains(output, "--- File: src/main/java/com/example/core/CoreService.java (Full File) ---") {
		t.Errorf("Expected output to contain file path header")
	}
	if !strings.Contains(output, "public class CoreService {}") {
		t.Errorf("Expected output to contain file content")
	}
	if !strings.Contains(output, "[TASK]\nAdd a method to CoreService") {
		t.Errorf("Expected output to contain the task")
	}
	if !strings.Contains(output, "--- File: <path/from/project_root> ---") {
		t.Errorf("Expected output to contain output constraints")
	}
}

func TestGenerateSchemaBIncludesFinalAnswerGuidance(t *testing.T) {
	formatter := NewFormatter()
	output, _ := formatter.GenerateSchemaB(&model.ProjectTopology{}, []ExtractionResult{
		{Path: "main.go", Content: "package main"},
	}, "what should I do next?")

	for _, want := range []string{
		"This is the final-answer step. Source context has already been extracted.",
		"Do NOT respond with FILE:, PREFIX:, or NEAR: lines; those selector operators are only for Prompt 1 responses.",
		"1. For updated or new files:",
		"--- File: <path/from/project_root> ---",
		"<full updated file contents>",
		"2. For explicit file deletion:",
		"--- Delete File: <path/from/project_root> ---",
		"3. For non-code responses: Just write the text normally.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected Prompt 2 to contain %q, got:\n%s", want, output)
		}
	}
}

func TestGenerateSchemaBTrimming(t *testing.T) {
	formatter := NewFormatter()
	formatter.MaxContextFileBytes = 20
	topology := &model.ProjectTopology{}

	// UTF-8: "hello " (6 bytes) + "世界" (6 bytes) + " " (1 byte) + "badger" (6 bytes) = 19 bytes
	content := "hello 世界 badger"
	extractions := []ExtractionResult{{Path: "f1", Content: content}}

	// Should not trim 19 bytes if limit is 20
	out, meta := formatter.GenerateSchemaB(topology, extractions, "query")
	if meta[0].Truncated {
		t.Error("Expected no truncation for 19 bytes with limit 20")
	}
	if !strings.Contains(out, content) {
		t.Error("Content should be intact")
	}

	// Now set limit to 10
	formatter.MaxContextFileBytes = 10
	out, meta = formatter.GenerateSchemaB(topology, extractions, "query")
	if !meta[0].Truncated {
		t.Error("Expected truncation for 19 bytes with limit 10")
	}
	if !strings.Contains(out, "[Truncated") {
		t.Error("Expected truncation marker")
	}
	if !utf8.ValidString(out) {
		t.Fatal("Expected truncated output to remain valid UTF-8")
	}

	// Check UTF-8 boundary safety: "hello " (6 bytes), the next 3 bytes are "世"
	// 10 / 2 = 5 bytes. content[:5] is "hello".
	// If it was in the middle of a rune, trimContent should handle it.
}

func TestNewFormatterUsesDefaultContextLimits(t *testing.T) {
	formatter := NewFormatter()

	if formatter.MaxContextFileBytes != defaults.MaxContextFileBytes {
		t.Fatalf("MaxContextFileBytes = %d, want %d", formatter.MaxContextFileBytes, defaults.MaxContextFileBytes)
	}
	if formatter.MaxTotalContextBytes != defaults.MaxTotalContextBytes {
		t.Fatalf("MaxTotalContextBytes = %d, want %d", formatter.MaxTotalContextBytes, defaults.MaxTotalContextBytes)
	}
}

func TestGenerateSchemaBTotalDropping(t *testing.T) {
	formatter := NewFormatter()
	formatter.MaxContextFileBytes = 0
	topology := &model.ProjectTopology{}
	query := "query"

	firstOnly := []ExtractionResult{
		{Path: "f1", Content: strings.Repeat("a", 600)},
	}
	firstSchema, _ := formatter.GenerateSchemaB(topology, firstOnly, query)
	formatter.MaxTotalContextBytes = len(firstSchema)

	extractions := []ExtractionResult{
		{Path: "f1", Content: strings.Repeat("a", 600)},
		{Path: "f2", Content: strings.Repeat("b", 600)},
	}

	out, meta := formatter.GenerateSchemaB(topology, extractions, query)

	if !strings.Contains(out, "Active Extractions: 1 files") {
		t.Errorf("Expected 1 file to be dropped, got: %s", out)
	}
	if !meta[1].Dropped {
		t.Error("Expected second file to be marked as dropped")
	}
	if !strings.Contains(out, "f1") || strings.Contains(out, "f2") {
		t.Error("Expected f1 to be present and f2 to be dropped")
	}
}

func TestGenerateSchemaBTotalDroppingRespectsFinalSize(t *testing.T) {
	formatter := NewFormatter()
	formatter.MaxContextFileBytes = 0
	formatter.MaxTotalContextBytes = 0
	topology := &model.ProjectTopology{}
	query := "query"
	firstOnly := []ExtractionResult{{Path: "f1", Content: strings.Repeat("a", 80)}}
	firstSchema, _ := formatter.GenerateSchemaB(topology, firstOnly, query)

	formatter.MaxTotalContextBytes = len(firstSchema)
	extractions := []ExtractionResult{
		{Path: "f1", Content: strings.Repeat("a", 80)},
		{Path: "f2", Content: strings.Repeat("b", 80)},
	}

	out, meta := formatter.GenerateSchemaB(topology, extractions, query)
	if len(out) > formatter.MaxTotalContextBytes {
		t.Fatalf("schema size = %d, want <= %d", len(out), formatter.MaxTotalContextBytes)
	}
	if !meta[1].Dropped {
		t.Fatal("Expected second file to be dropped")
	}
}

func TestGenerateSchemaBTotalDroppingHandlesDuplicatePaths(t *testing.T) {
	formatter := NewFormatter()
	formatter.MaxContextFileBytes = 0
	formatter.MaxTotalContextBytes = 0
	topology := &model.ProjectTopology{}
	query := "query"
	firstOnly := []ExtractionResult{{Path: "same.go", Content: "keep me"}}
	firstSchema, _ := formatter.GenerateSchemaB(topology, firstOnly, query)

	formatter.MaxTotalContextBytes = len(firstSchema)
	extractions := []ExtractionResult{
		{Path: "same.go", Content: "keep me"},
		{Path: "same.go", Content: strings.Repeat("drop me", 40)},
	}

	out, meta := formatter.GenerateSchemaB(topology, extractions, query)
	if meta[0].Dropped {
		t.Fatal("Expected first duplicate path extraction to remain active")
	}
	if !meta[1].Dropped {
		t.Fatal("Expected second duplicate path extraction to be dropped")
	}
	if !strings.Contains(out, "keep me") || strings.Contains(out, "drop me") {
		t.Fatalf("duplicate path output kept wrong content:\n%s", out)
	}
}

func TestGenerateSchemaBDisableTrimming(t *testing.T) {
	formatter := NewFormatter()
	formatter.MaxContextFileBytes = 0
	formatter.MaxTotalContextBytes = 0
	topology := &model.ProjectTopology{}

	content := strings.Repeat("x", 20000)
	extractions := []ExtractionResult{{Path: "f1", Content: content}}

	out, meta := formatter.GenerateSchemaB(topology, extractions, "query")
	if meta[0].Truncated {
		t.Error("Expected no truncation when limit is 0")
	}
	if !strings.Contains(out, content) {
		t.Error("Content should be fully preserved")
	}
}

func TestFormatterCustomInstructions(t *testing.T) {
	formatter := NewFormatter()
	custom := PromptInstructions{
		SchemaAConstraint: "CUSTOM A: %s",
		SchemaBConstraint: "CUSTOM B: %s",
	}
	formatter.Instructions = custom

	topology := &model.ProjectTopology{}
	query := "hello"

	outA := formatter.GenerateSchemaA(topology, query)
	if !strings.Contains(outA, "CUSTOM A: hello") {
		t.Errorf("Expected custom A constraint, got:\n%s", outA)
	}

	outB, _ := formatter.GenerateSchemaB(topology, []ExtractionResult{{Path: "f1", Content: "c1"}}, query)
	if !strings.Contains(outB, "CUSTOM B: hello") {
		t.Errorf("Expected custom B constraint, got:\n%s", outB)
	}
}
