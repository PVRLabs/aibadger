package promptpolicy

import (
	"path/filepath"
	"strings"
)

// SensitivePathNames captures obvious secret-bearing paths that must never be
// surfaced in prompt text. The list is intentionally conservative.
var SensitivePathNames = []string{
	".env",
	".env.*",
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"credentials",
	"credentials.json",
	"secret.json",
	"secrets.json",
	"id_rsa",
	"id_dsa",
	"id_ecdsa",
	"id_ed25519",
	"*_rsa",
	"*_dsa",
	"*_ecdsa",
	"*_ed25519",
	".aws/credentials",
	".aws/config",
	".gcp/credentials.json",
	".azure/",
	".npmrc",
	".pypirc",
	".netrc",
	"*.kubeconfig",
}

// IsSensitivePath reports whether a repo-relative path is obviously sensitive.
func IsSensitivePath(relPath string) bool {
	path := strings.ToLower(filepath.ToSlash(filepath.Clean(relPath)))
	if path == "." || path == "" {
		return false
	}

	base := filepath.Base(path)
	if matchesSensitiveBase(base) {
		return true
	}

	parts := strings.Split(path, "/")
	for i, part := range parts {
		switch part {
		case ".azure":
			return true
		case ".aws":
			if i+1 < len(parts) && (parts[i+1] == "credentials" || parts[i+1] == "config") {
				return true
			}
		case ".gcp":
			if i+1 < len(parts) && parts[i+1] == "credentials.json" {
				return true
			}
		}
	}

	return false
}

func matchesSensitiveBase(base string) bool {
	switch base {
	case ".env", ".npmrc", ".pypirc", ".netrc",
		"credentials", "credentials.json", "secret.json", "secrets.json",
		"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519":
		return true
	}

	if strings.HasPrefix(base, ".env.") {
		return true
	}
	if strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") ||
		strings.HasSuffix(base, ".p12") || strings.HasSuffix(base, ".pfx") ||
		strings.HasSuffix(base, ".kubeconfig") {
		return true
	}
	if strings.HasSuffix(base, "_rsa") || strings.HasSuffix(base, "_dsa") ||
		strings.HasSuffix(base, "_ecdsa") || strings.HasSuffix(base, "_ed25519") {
		return true
	}

	return false
}
