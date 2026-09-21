package diagnose

import "testing"

func TestParsersReturnOnlyNormalizedVersions(t *testing.T) {
	tests := []struct {
		name  string
		parse func(string) (string, bool)
		input string
		want  string
	}{
		{name: "git", parse: parseGit, input: "git version 2.51.0.windows.1", want: "2.51.0.windows.1"},
		{name: "go", parse: parseGo, input: "go version go1.27.1 linux/amd64", want: "1.27.1"},
		{name: "go release candidate", parse: parseGo, input: "go version go1.28rc1 linux/amd64", want: "1.28rc1"},
		{name: "go beta", parse: parseGo, input: "go version go1.29beta2 linux/amd64", want: "1.29beta2"},
		{name: "java stderr", parse: parseJava, input: "openjdk version \"25.0.2\" 2026-01-01\nOpenJDK Runtime /private/sdk", want: "25.0.2"},
		{name: "node", parse: parseSimpleVersion, input: "v25.9.0\n", want: "25.9.0"},
		{name: "pip path", parse: parsePip, input: "pip 25.2 from /Users/private/site-packages (python 3.14)", want: "25.2"},
		{name: "dotnet SDKs", parse: parseDotnetSDKs, input: "8.0.414 [/usr/local/share/dotnet/sdk]\n10.0.100-preview.7.25380.108 [C:\\Program Files\\dotnet\\sdk]", want: "8.0.414, 10.0.100-preview.7.25380.108"},
		{name: "compiler", parse: parseCompiler, input: "Apple clang version 17.0.0 (clang-1700.0.13.3)", want: "Clang 17.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := test.parse(test.input)
			if !ok || got != test.want {
				t.Fatalf("parse(%q) = %q, %v; want %q, true", test.input, got, ok, test.want)
			}
		})
	}
}

func TestSimpleVersionRejectsArbitraryOutput(t *testing.T) {
	for _, input := range []string{"downloaded update 11.6.0", "/private/sdk/10.0.100", "11.6.0\n/private/path"} {
		if got, ok := parseSimpleVersion(input); ok {
			t.Fatalf("parseSimpleVersion(%q) = %q, true; want rejection", input, got)
		}
	}
}
