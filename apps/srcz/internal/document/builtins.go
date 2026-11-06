package document

// Builtins is the explicit composition point for file families. Registration
// only decides presentation; files outside these families use the plain-text
// fallback. Each family's Language names a Highlights language explicitly, or
// "text" for none; the browser highlights Source and Format (decision 0029).
func Builtins() *Registry {
	registry, err := NewRegistry(
		source("text", "Plain text", "text", []string{".txt", ".log", ".adoc"}, ".gitignore", ".dockerignore"),
		newMarkdown(),
		newMermaid(),
		NewJSON(),
		NewYAML(),
		CodeHandler{Definition: Type{ID: "svg", Label: "SVG", Extensions: []string{".svg"}, Language: "xml", PreviewKind: PreviewImage, PreviewLabel: "Preview", DefaultMode: "preview"}},
		CodeHandler{Definition: Type{ID: "image", Label: "Image", Extensions: []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico", ".bmp"}, PreviewKind: PreviewImage, PreviewLabel: "Preview", DefaultMode: "preview"}},
		// Text and configuration.
		source("rst", "reStructuredText", "text", []string{".rst"}),
		source("diff", "Diff", "diff", []string{".diff", ".patch"}),
		source("toml", "TOML", "toml", []string{".toml"}),
		source("ini", "INI", "text", []string{".ini", ".cfg", ".conf"}),
		source("xml", "XML", "xml", []string{".xml"}),
		source("properties", "Properties", "text", []string{".properties"}),
		source("hcl", "HCL", "terraform", []string{".hcl"}),
		source("terraform", "Terraform", "terraform", []string{".tf"}),
		// Shell and build.
		source("shell", "Shell", "bash", []string{".sh", ".bash", ".zsh"}, ".env"),
		source("fish", "Fish", "text", []string{".fish"}),
		source("powershell", "PowerShell", "powershell", []string{".ps1"}),
		source("dockerfile", "Dockerfile", "dockerfile", nil, "Dockerfile"),
		source("makefile", "Makefile", "makefile", nil, "Makefile", "GNUmakefile"),
		// Code.
		source("go", "Go", "go", []string{".go"}),
		source("python", "Python", "python", []string{".py"}),
		NewJavaScript(),
		source("typescript", "TypeScript", "ts", []string{".ts"}),
		source("tsx", "TSX", "tsx", []string{".tsx"}),
		source("jsx", "JSX", "jsx", []string{".jsx"}),
		source("rust", "Rust", "rust", []string{".rs"}),
		source("java", "Java", "java", []string{".java"}),
		source("kotlin", "Kotlin", "kotlin", []string{".kt"}),
		source("c", "C", "c", []string{".c", ".h"}),
		source("cpp", "C++", "cpp", []string{".cpp", ".hpp", ".cc"}),
		source("ruby", "Ruby", "ruby", []string{".rb"}),
		source("php", "PHP", "php", []string{".php"}),
		source("lua", "Lua", "lua", []string{".lua"}),
		source("swift", "Swift", "swift", []string{".swift"}),
		source("sql", "SQL", "sql", []string{".sql"}),
		NewCSS(),
		source("scss", "SCSS", "scss", []string{".scss"}),
		NewHTML(),
	)
	if err != nil {
		// Static built-in definitions are programmer-owned.
		panic(err)
	}
	return registry
}

func source(id, label, language string, extensions []string, filenames ...string) Handler {
	return sourceCode(id, label, language, extensions, filenames...)
}

func sourceCode(id, label, language string, extensions []string, filenames ...string) CodeHandler {
	return CodeHandler{Definition: Type{ID: id, Label: label, Extensions: extensions, Filenames: filenames, Language: language, DefaultMode: "source"}}
}
