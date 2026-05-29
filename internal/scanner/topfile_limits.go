package scanner

const (
	maxPackageTopFiles     = 3
	maxRootPackageTopFiles = 10
)

func packageTopFileLimit(packagePath string, nonRootLimit int) int {
	if packagePath == "" {
		return maxRootPackageTopFiles
	}
	return nonRootLimit
}

func moduleTopFileLimit(modulePath string, packageLimit int) int {
	if modulePath == "" && packageLimit < maxRootPackageTopFiles {
		return maxRootPackageTopFiles
	}
	return packageLimit
}
