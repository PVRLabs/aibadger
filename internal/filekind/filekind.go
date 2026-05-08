package filekind

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
)

const sniffBytes = 4096

// Classify returns a compact file kind for topology and extraction safety.
func Classify(path string) string {
	kind := classifyByName(path)
	if kind != "" {
		return kind
	}
	return classifyByContent(path)
}

func classifyByName(path string) string {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(name))

	switch ext {
	case ".go", ".java", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs",
		".cpp", ".c", ".h", ".hpp", ".rs", ".rb", ".php", ".cs", ".kt", ".swift",
		".html", ".htm", ".css", ".scss", ".sass", ".json", ".yaml", ".yml",
		".toml", ".xml", ".ini", ".conf", ".properties", ".md", ".txt",
		".sh", ".bash", ".zsh", ".sql":
		return model.FileKindSource
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico",
		".woff", ".woff2", ".ttf", ".otf", ".mp3", ".mp4", ".mov", ".webm":
		return model.FileKindAsset
	case ".exe", ".dll", ".so", ".dylib", ".class", ".jar", ".war", ".ear",
		".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar",
		".pdf", ".wasm", ".bin":
		return model.FileKindBinary
	}

	switch name {
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"vite.config.js", "vite.config.ts", "next.config.js", "next.config.ts",
		"webpack.config.js", "tsconfig.json", "jsconfig.json", "index.html",
		"readme", "readme.md", "agents.md", "license",
		"makefile", "dockerfile", "taskfile.yml", "justfile",
		".gitignore", ".dockerignore":
		return model.FileKindSource
	}

	return ""
}

func classifyByContent(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return model.FileKindSource
	}
	defer file.Close()

	buf := make([]byte, sniffBytes)
	n, readErr := file.Read(buf)
	if readErr != nil && n == 0 {
		return model.FileKindSource
	}
	if bytesLookBinary(buf[:n]) {
		return model.FileKindBinary
	}
	return model.FileKindSource
}

func bytesLookBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
