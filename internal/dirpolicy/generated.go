// Package dirpolicy defines shared discovery exclusions for generated directories.
package dirpolicy

import "strings"

// GeneratedNames lists exact directory components, never filename substrings.
// Keep configuration and authored source directories out of this policy.
func GeneratedNames() []string {
	return strings.Fields(`
 .playwright-cli .playwright .vscode-test .vscode-test-web .agent-logs playwright-report blob-report test-results
 testresults allure-results allure-report htmlcov lcov-report storybook-static
 coverage .nyc_output
 .cache .gocache .npm .pnpm-store .yarn-cache .parcel-cache .eslintcache
 .stylelintcache .sass-cache .vite .vitest .turbo .nx
 __pycache__ __pypackages__ .pytest_cache .mypy_cache .ruff_cache
 .hypothesis .ipynb_checkpoints .tox .nox .venv venv
 .next .nuxt .output .svelte-kit .angular .astro .docusaurus .expo
 .gradle .kotlin .m2 .bloop .metals .bsp
 .dart_tool .pub-cache .pub .build .cxx .externalnativebuild cmakefiles .vs
 .terraform .terragrunt-cache .serverless .aws-sam
 node_modules bower_components build dist target
 .tmp .temp tmp temp
 `)
}

var generated = func() map[string]bool {
	result := make(map[string]bool)
	for _, name := range GeneratedNames() {
		result[name] = true
	}
	return result
}()

// SkipGenerated matches complete components case-insensitively. CMake's IDE
// build-directory prefix is a conventional generated variant.
func SkipGenerated(name string) bool {
	lower := strings.ToLower(name)
	return generated[lower] || strings.HasPrefix(lower, "cmake-build-")
}
