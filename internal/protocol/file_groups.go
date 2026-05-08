package protocol

import "github.com/PVRLabs/aibadger/internal/filegroups"

func isArchitectureLikeDoc(base string) bool {
	return filegroups.IsArchitectureLikeDoc(base)
}

func isPlanningArtifactDoc(base string) bool {
	return filegroups.IsPlanningArtifactDoc(base)
}

func isShallowDocumentationPath(lowerPath string) bool {
	return filegroups.IsShallowDocumentationPath(lowerPath)
}

func isKnownStaticWebPath(lowerPath string) bool {
	return filegroups.IsKnownStaticWebPath(lowerPath)
}
