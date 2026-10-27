package codemap

type languageStats struct {
	Lines int `json:"lines"`
	Files int `json:"files"`
}

type sourceFile struct {
	Name  string `json:"name"`
	Lines int    `json:"lines"`
}

// directory contains metadata only. Children omit zero-line geometry after
// recursive accounting; DirectByLang must never be inferred from Children.
type directory struct {
	GeneratorVersion string                   `json:"generatorVersion,omitempty"`
	Reproduction     *reproduction            `json:"reproduction,omitempty"`
	Name             string                   `json:"name"`
	Path             string                   `json:"path"`
	Lines            int                      `json:"lines"`
	Files            int                      `json:"files"`
	DirectLines      int                      `json:"directLines"`
	FileList         []sourceFile             `json:"fileList"`
	DirectFiles      int                      `json:"directFiles"`
	Languages        map[string]languageStats `json:"languages"`
	DirectByLang     map[string]languageStats `json:"directLanguages"`
	Children         []*directory             `json:"children"`
}
