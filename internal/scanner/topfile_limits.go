package scanner

const (
	maxPackageTopFiles     = 3
	maxRootPackageTopFiles = 10
)

func effectiveTopFileLimit(values ...int) int {
	if len(values) > 0 && values[0] >= 3 && values[0] <= 50 {
		return values[0]
	}
	return maxPackageTopFiles
}

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

// Generic fallback only increases its existing ten-file package budget.
func genericPackageTopFileLimit(path string, values ...int) int {
	if path == "" || len(values) == 0 || values[0] == 0 {
		return maxGenericPackageFiles
	}
	limit := effectiveTopFileLimit(values...)
	if limit < maxGenericPackageFiles {
		return maxGenericPackageFiles
	}
	return limit
}
