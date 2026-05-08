package scanner

// This file owns stack and framework signal detection.

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func detectedStack(root string) []string {
	found := make(map[string]bool)
	nodeSignals := detectNodeStacks(root)
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name(), commonIgnoredDirs) {
				return filepath.SkipDir
			}
			return nil
		}
		switch d.Name() {
		case "pom.xml":
			found["Maven"] = true
		case "build.gradle", "build.gradle.kts":
			found["Gradle"] = true
		case "go.mod":
			found["Go Modules"] = true
		case "package.json":
			found["Node.js"] = true
		}
		return nil
	})
	for name, ok := range nodeSignals {
		if ok {
			found[name] = true
		}
	}
	stackOrder := []string{"Maven", "Gradle", "Go Modules", "Node.js", "React", "Next.js", "Vue", "Vite", "NestJS"}
	var stack []string
	for _, name := range stackOrder {
		if found[name] {
			stack = append(stack, name)
		}
	}
	return stack
}

type nodeDependencySet struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func detectNodeStacks(root string) map[string]bool {
	signals := make(map[string]bool)
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name(), commonIgnoredDirs) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "package.json" {
			return nil
		}

		deps, ok := readNodeDependencySet(path)
		if !ok {
			return nil
		}

		moduleRoot := filepath.Dir(path)
		if hasNextTopologySignal(deps, moduleRoot) {
			signals["Next.js"] = true
		}
		if hasReactTopologySignal(deps, moduleRoot) {
			signals["React"] = true
		}
		if hasVueTopologySignal(deps, moduleRoot) {
			signals["Vue"] = true
		}
		if hasViteTopologySignal(deps, moduleRoot) {
			signals["Vite"] = true
		}
		if hasNestTopologySignal(deps, moduleRoot) {
			signals["NestJS"] = true
		}
		return nil
	})
	return signals
}

func readNodeDependencySet(path string) (nodeDependencySet, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nodeDependencySet{}, false
	}
	var deps nodeDependencySet
	if err := json.Unmarshal(data, &deps); err != nil {
		return nodeDependencySet{}, false
	}
	return deps, true
}

func hasNodeDependency(deps nodeDependencySet, name string) bool {
	if hasKey(deps.Dependencies, name) {
		return true
	}
	return hasKey(deps.DevDependencies, name)
}

func nodePackageDependencies(pkg nodePackageJSON) nodeDependencySet {
	return nodeDependencySet{
		Dependencies:    pkg.Dependencies,
		DevDependencies: pkg.DevDependencies,
	}
}

func hasReactTopologySignal(deps nodeDependencySet, root string) bool {
	return hasNodeDependency(deps, "react") &&
		hasAnyPath(root, "src/App.jsx", "src/App.tsx", "App.jsx", "App.tsx", "components", "hooks")
}

func hasReactOverviewSignal(deps nodeDependencySet, root string) bool {
	return hasNodeDependency(deps, "react") && hasAnyPath(root,
		"App.jsx", "App.tsx", "src/App.jsx", "src/App.tsx",
		"components", "hooks", filepath.Join("src", "components"), filepath.Join("src", "hooks"),
	)
}

func hasNextTopologySignal(deps nodeDependencySet, root string) bool {
	return hasNextSignal(deps, root)
}

func hasNextOverviewSignal(deps nodeDependencySet, root string) bool {
	return hasNextSignal(deps, root)
}

func hasNextSignal(deps nodeDependencySet, root string) bool {
	return hasNodeDependency(deps, "next") && hasAnyPath(root,
		"next.config.js", "next.config.ts", "next.config.mjs", "next.config.cjs",
		"pages", "app",
	)
}

func hasVueTopologySignal(deps nodeDependencySet, root string) bool {
	return hasNodeDependency(deps, "vue") && hasAnyPath(root, "src/App.vue", "App.vue", "components")
}

func hasVueOverviewSignal(deps nodeDependencySet, root string) bool {
	return hasNodeDependency(deps, "vue") && hasAnyPath(root,
		"src/main.ts", "src/main.js", "main.ts", "main.js",
		"src/App.vue", "App.vue", "components", filepath.Join("src", "components"),
	)
}

func hasViteTopologySignal(deps nodeDependencySet, root string) bool {
	return hasViteSignal(deps, root)
}

func hasViteOverviewSignal(deps nodeDependencySet, root string) bool {
	return hasViteSignal(deps, root)
}

func hasViteSignal(deps nodeDependencySet, root string) bool {
	return hasNodeDependency(deps, "vite") && hasAnyPath(root,
		"vite.config.js", "vite.config.ts", "vite.config.mjs", "vite.config.cjs", "index.html",
	)
}

func hasNestTopologySignal(deps nodeDependencySet, root string) bool {
	return hasNestDependency(deps) &&
		hasAnyPath(root, "nest-cli.json", filepath.Join("src", "main.ts"), filepath.Join("src", "main.js"))
}

func hasNestOverviewSignal(deps nodeDependencySet, root string) bool {
	return hasNestDependency(deps) &&
		hasAnyPath(root, "nest-cli.json", filepath.Join("src", "main.ts"), filepath.Join("src", "main.js"), "apps", "libs")
}

func hasNestDependency(deps nodeDependencySet) bool {
	return hasNodeDependency(deps, "@nestjs/core") || hasNodeDependency(deps, "@nestjs/common")
}

func hasAnyPath(root string, relPaths ...string) bool {
	for _, relPath := range relPaths {
		info, err := os.Stat(filepath.Join(root, relPath))
		if err == nil && info != nil {
			return true
		}
	}
	return false
}
