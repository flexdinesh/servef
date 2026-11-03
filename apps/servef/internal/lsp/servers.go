package lsp

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Server describes a deliberately supported language-server process.
type Server struct {
	// WorkspaceFiles are base-name globs for configuration change notifications.
	WorkspaceFiles []string
	Aliases        []string
	Name           string
	Args           []string
	Language       string
	Families       map[string]string
	Install        []Install
	Docs           string
	// NativeTypeScript selects TypeScript 7's native LSP before the classic adapter.
	NativeTypeScript bool
	// NodePackage identifies npm launchers on hosts that cannot execute shebangs.
	NodePackage string
}

// Install is host-specific guidance, never an automatically executed command.
// Command is empty for downloads that require following the linked documentation.
type Install struct {
	Label         string   `json:"label"`
	Command       string   `json:"command"`
	Prerequisites []string `json:"prerequisites,omitempty"`
	Docs          string   `json:"docs,omitempty"`
}

// ResolvedServer identifies the selected executable without starting it.
type ResolvedServer struct {
	Path string
	Name string
	Args []string
}

// Resolve checks project-local tools before host tools. Discovery reads files and
// PATH only; status requests never run a shell, installer, or language server.
func (s Server) Resolve(root string, lookup func(string) (string, bool)) (ResolvedServer, bool) {
	names := append([]string{s.Name}, s.Aliases...)
	for dir := root; dir != ""; dir = parentDir(dir) {
		if s.NativeTypeScript {
			for _, name := range []string{"typescript", "@typescript/native-preview"} {
				if resolved, ok := nativeTypeScript(filepath.Join(dir, "node_modules", filepath.FromSlash(name)), lookup); ok {
					return resolved, true
				}
			}
		}
		if runtime.GOOS == "windows" && s.NodePackage != "" {
			if process, ok := s.nodeProcess(filepath.Join(dir, "node_modules", filepath.FromSlash(s.NodePackage)), lookup); ok {
				return process, true
			}
			continue
		}
		for _, name := range names {
			if path, ok := executable(filepath.Join(dir, "node_modules", ".bin", name)); ok {
				return ResolvedServer{Path: path, Name: name, Args: s.Args}, true
			}
		}
	}
	if s.NativeTypeScript {
		for _, name := range []string{"tsc", "tsgo"} {
			if path, ok := lookup(name); ok {
				for _, pkg := range []string{"typescript", "@typescript/native-preview"} {
					if resolved, ok := nativeTypeScript(filepath.Join(filepath.Dir(path), "node_modules", filepath.FromSlash(pkg)), lookup); ok {
						return resolved, true
					}
				}
				if real, err := filepath.EvalSymlinks(path); err == nil {
					for dir := filepath.Dir(real); dir != ""; dir = parentDir(dir) {
						if resolved, ok := nativeTypeScript(dir, lookup); ok {
							return resolved, true
						}
					}
				}
			}
		}
	}
	for _, name := range names {
		if path, ok := lookup(name); ok {
			if runtime.GOOS == "windows" && s.NodePackage != "" {
				return s.nodeProcess(filepath.Join(filepath.Dir(path), "node_modules", filepath.FromSlash(s.NodePackage)), lookup)
			}
			return ResolvedServer{Path: path, Name: name, Args: s.Args}, true
		}
	}
	return ResolvedServer{}, false
}

func parentDir(dir string) string {
	parent := filepath.Dir(dir)
	if parent == dir {
		return ""
	}
	return parent
}

func nativeTypeScript(dir string, lookup func(string) (string, bool)) (ResolvedServer, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ResolvedServer{}, false
	}
	var pkg struct {
		Name    string
		Version string
		Bin     map[string]string
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ResolvedServer{}, false
	}
	major, _ := strconv.Atoi(strings.Split(pkg.Version, ".")[0])
	if pkg.Name != "@typescript/native-preview" && (pkg.Name != "typescript" || major < 7) {
		return ResolvedServer{}, false
	}
	for _, name := range []string{"tsc", "tsgo"} {
		if bin := pkg.Bin[name]; bin != "" {
			if path, ok := executable(filepath.Join(dir, filepath.FromSlash(bin))); ok {
				return scriptProcess(path, name, []string{"--lsp", "--stdio"}, lookup)
			}
		}
	}
	return ResolvedServer{}, false
}

// npm launchers are JavaScript scripts. Windows requires node.exe rather than
// attempting to launch a .cmd shim through a shell.
func scriptProcess(path, name string, args []string, lookup func(string) (string, bool)) (ResolvedServer, bool) {
	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(path), ".exe") {
		node, ok := lookup("node")
		if !ok {
			return ResolvedServer{}, false
		}
		return ResolvedServer{Path: node, Name: name, Args: append([]string{path}, args...)}, true
	}
	return ResolvedServer{Path: path, Name: name, Args: args}, true
}

func (s Server) nodeProcess(dir string, lookup func(string) (string, bool)) (ResolvedServer, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ResolvedServer{}, false
	}
	var pkg struct{ Bin map[string]string }
	if json.Unmarshal(data, &pkg) != nil {
		return ResolvedServer{}, false
	}
	bin := pkg.Bin[s.Name]
	if bin == "" {
		return ResolvedServer{}, false
	}
	path, ok := executable(filepath.Join(dir, filepath.FromSlash(bin)))
	if !ok {
		return ResolvedServer{}, false
	}
	return scriptProcess(path, s.Name, s.Args, lookup)
}

// Builtins lists the supported language servers with guidance for this host.
func Builtins() []Server { return builtinServers(detectHost()) }

type host struct {
	os, distro string
	tools      map[string]bool
}

func detectHost() host {
	h := host{os: runtime.GOOS, tools: map[string]bool{}}
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if key, value, ok := strings.Cut(line, "="); ok && (key == "ID" || key == "ID_LIKE") {
				h.distro += " " + strings.Trim(value, "\"'")
			}
		}
	}
	for _, name := range []string{"apt-get", "dnf", "pacman", "brew", "scoop"} {
		_, h.tools[name] = lookPath(name)
	}
	return h
}

func builtinServers(h host) []Server {
	npm := func(command string) Install {
		return Install{Label: "npm", Command: command, Prerequisites: []string{"Node.js and npm on PATH"}}
	}
	manual := func(label, docs string, requirements ...string) Install {
		return Install{Label: label, Docs: docs, Prerequisites: requirements}
	}
	servers := []Server{
		{Name: "gopls", WorkspaceFiles: []string{"go.mod", "go.sum", "go.work", "go.work.sum"}, Language: "Go", Families: map[string]string{"go": "go"}, Install: []Install{{Label: "Go", Command: "go install golang.org/x/tools/gopls@latest", Prerequisites: []string{"Go on PATH; add GOBIN or ~/go/bin to PATH"}}}, Docs: "https://go.dev/gopls/"},
		{Name: "typescript-language-server", WorkspaceFiles: []string{"tsconfig*.json", "jsconfig*.json", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock"}, NodePackage: "typescript-language-server", Args: []string{"--stdio"}, Language: "JavaScript and TypeScript", NativeTypeScript: true, Families: map[string]string{"javascript": "javascript", "jsx": "javascriptreact", "typescript": "typescript", "tsx": "typescriptreact"}, Install: []Install{npm("npm install -g typescript@latest"), npm("npm install -g typescript-language-server typescript@6")}, Docs: "https://github.com/microsoft/TypeScript"},
		{Name: "pyright-langserver", WorkspaceFiles: []string{"pyrightconfig.json", "pyproject.toml", "setup.cfg", "requirements*.txt", "uv.lock", "poetry.lock", "Pipfile", "Pipfile.lock"}, NodePackage: "pyright", Args: []string{"--stdio"}, Language: "Python", Families: map[string]string{"python": "python"}, Install: []Install{npm("npm install -g pyright")}, Docs: "https://github.com/microsoft/pyright"},
		{Name: "rust-analyzer", WorkspaceFiles: []string{"Cargo.toml", "Cargo.lock", "rust-toolchain", "rust-toolchain.toml", "config", "config.toml"}, Language: "Rust", Families: map[string]string{"rust": "rust"}, Install: []Install{{Label: "rustup", Command: "rustup component add rust-analyzer rust-src", Prerequisites: []string{"Rust and rustup on PATH"}}}, Docs: "https://rust-analyzer.github.io/book/installation.html"},
		{Name: "clangd", WorkspaceFiles: []string{"compile_commands.json", "compile_flags.txt", ".clangd"}, Language: "C and C++", Families: map[string]string{"c": "c", "cpp": "cpp"}, Install: []Install{manual("LLVM installation", "https://clangd.llvm.org/installation", "Provide compile_commands.json for project build flags")}, Docs: "https://clangd.llvm.org/installation"},
		{Name: "jdtls", WorkspaceFiles: []string{"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "gradle.properties", ".project", ".classpath"}, Language: "Java", Families: map[string]string{"java": "java"}, Install: []Install{manual("Eclipse JDT LS download", "https://github.com/eclipse-jdtls/eclipse.jdt.ls#installation", "JDK 21+ on PATH or JAVA_HOME", "Python 3.9+ for the jdtls wrapper; add its bin directory to PATH")}, Docs: "https://github.com/eclipse-jdtls/eclipse.jdt.ls"},
		{Name: "kotlin-lsp", WorkspaceFiles: []string{"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "gradle.properties"}, Aliases: []string{"intellij-server", "kotlin-lsp.sh"}, Args: []string{"--stdio"}, Language: "Kotlin", Families: map[string]string{"kotlin": "kotlin"}, Install: []Install{manual("Kotlin LSP download", "https://github.com/Kotlin/kotlin-lsp#install-kotlin-lsp-cli", "JDK 25+; extract the standalone release and add its bin directory (intellij-server) to PATH")}, Docs: "https://github.com/Kotlin/kotlin-lsp"},
		{Name: "ruby-lsp", WorkspaceFiles: []string{"Gemfile", "Gemfile.lock", ".ruby-version", ".ruby-lsp.yml"}, Language: "Ruby", Families: map[string]string{"ruby": "ruby"}, Install: []Install{{Label: "RubyGems", Command: "gem install ruby-lsp", Prerequisites: []string{"Project Ruby and RubyGems on PATH; install project dependencies with Bundler"}}}, Docs: "https://shopify.github.io/ruby-lsp/"},
		{Name: "phpactor", WorkspaceFiles: []string{"composer.json", "composer.lock", ".phpactor.json", ".phpactor.yml"}, Args: []string{"language-server"}, Language: "PHP", Families: map[string]string{"php": "php"}, Install: []Install{manual("Phpactor PHAR download", "https://phpactor.readthedocs.io/en/master/usage/standalone.html", "PHP 8.2+ on PATH; install the executable as phpactor on PATH")}, Docs: "https://phpactor.readthedocs.io/en/master/usage/standalone.html"},
		{Name: "lua-language-server", WorkspaceFiles: []string{".luarc.json", ".luarc.jsonc"}, Language: "Lua", Families: map[string]string{"lua": "lua"}, Install: []Install{manual("LuaLS download", "https://luals.github.io/#install", "Install a release for this operating system and add its launcher to PATH")}, Docs: "https://luals.github.io/"},
		{Name: "sourcekit-lsp", WorkspaceFiles: []string{"Package.swift", "Package.resolved", "compile_commands.json"}, Language: "Swift", Families: map[string]string{"swift": "swift"}, Install: []Install{manual("Swift toolchain", "https://www.swift.org/install/", "Install the Swift toolchain and add its bin directory to PATH")}, Docs: "https://github.com/swiftlang/sourcekit-lsp"},
		{Name: "vscode-css-language-server", WorkspaceFiles: []string{"package.json"}, NodePackage: "vscode-langservers-extracted", Args: []string{"--stdio"}, Language: "CSS and SCSS", Families: map[string]string{"css": "css", "scss": "scss"}, Install: []Install{npm("npm install -g vscode-langservers-extracted")}, Docs: "https://github.com/hrsh7th/vscode-langservers-extracted"},
	}
	for i := range servers {
		s := &servers[i]
		if s.NativeTypeScript {
			s.Install[0].Label = "TypeScript 7+ native LSP"
			s.Install[1].Label = "TypeScript 6 adapter"
			s.Install[1].Docs = "https://github.com/typescript-language-server/typescript-language-server"
		}
		if h.os == "darwin" && h.tools["brew"] {
			packages := map[string]string{"gopls": "gopls", "rust-analyzer": "rust-analyzer", "clangd": "llvm", "jdtls": "jdtls", "kotlin-lsp": "--cask kotlin-lsp", "lua-language-server": "lua-language-server"}
			if pkg := packages[s.Name]; pkg != "" {
				recipe := Install{Label: "Homebrew", Command: "brew install " + pkg, Prerequisites: []string{"Homebrew on PATH"}}
				if s.Name == "kotlin-lsp" {
					recipe.Docs = "https://formulae.brew.sh/cask/kotlin-lsp"
					recipe.Prerequisites = append(recipe.Prerequisites, "JDK 25+")
				}
				s.Install = append([]Install{recipe}, s.Install...)
			}
		}
		if h.os == "linux" && s.Name == "clangd" {
			for _, family := range []struct{ distro, tool, command string }{{"debian ubuntu", "apt-get", "sudo apt-get install clangd"}, {"fedora rhel", "dnf", "sudo dnf install clang-tools-extra"}, {"arch", "pacman", "sudo pacman -S clang"}} {
				for _, distro := range strings.Fields(family.distro) {
					if h.tools[family.tool] && containsWord(h.distro, distro) {
						s.Install = append([]Install{{Label: family.tool, Command: family.command, Prerequisites: []string{"Administrator access"}}}, s.Install...)
						break
					}
				}
			}
		}
		if h.os == "darwin" && s.Name == "sourcekit-lsp" {
			s.Install = append([]Install{{Label: "Xcode Command Line Tools", Command: "xcode-select --install", Prerequisites: []string{"macOS; complete the installer dialog"}}}, s.Install...)
		}
		if h.os == "windows" && s.Name == "lua-language-server" && h.tools["scoop"] {
			s.Install = append([]Install{{Label: "Scoop", Command: "scoop install lua-language-server", Prerequisites: []string{"Scoop on PATH"}}}, s.Install...)
		}
		for j := range s.Install {
			if s.Install[j].Docs == "" {
				s.Install[j].Docs = s.Docs
			}
		}
	}
	return servers
}

func containsWord(words, word string) bool {
	for _, candidate := range strings.Fields(words) {
		if candidate == word {
			return true
		}
	}
	return false
}

// installDirs covers common launcher environments without spawning discovery commands.
func installDirs() []string {
	var dirs []string
	if bin := os.Getenv("GOBIN"); bin != "" {
		dirs = append(dirs, bin)
	}
	if path := os.Getenv("GOPATH"); path != "" {
		dirs = append(dirs, filepath.Join(filepath.SplitList(path)[0], "bin"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, dir := range []string{"go/bin", ".local/bin", ".npm-global/bin", ".bun/bin", ".volta/bin", ".cargo/bin", ".local/share/mise/shims", ".asdf/shims", ".rbenv/shims", ".config/composer/vendor/bin", ".composer/vendor/bin"} {
			dirs = append(dirs, filepath.Join(home, filepath.FromSlash(dir)))
		}
		if runtime.GOOS == "windows" {
			dirs = append(dirs, filepath.Join(home, "AppData", "Roaming", "npm"), filepath.Join(home, "scoop", "shims"))
		}
	}
	for _, name := range []string{"PNPM_HOME", "NVM_BIN"} {
		if dir := os.Getenv(name); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	if runtime.GOOS == "darwin" {
		dirs = append(dirs, "/opt/homebrew/bin", "/opt/homebrew/opt/llvm/bin", "/usr/local/opt/llvm/bin", "/Library/Developer/CommandLineTools/usr/bin", "/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin")
	}
	return append(dirs, "/usr/local/bin")
}

func executable(path string) (string, bool) {
	candidates := []string{path}
	if runtime.GOOS == "windows" {
		for _, extension := range filepath.SplitList(strings.ReplaceAll(os.Getenv("PATHEXT"), ";", string(os.PathListSeparator))) {
			candidates = append(candidates, path+strings.ToLower(extension))
		}
		candidates = append(candidates, path+".exe", path+".cmd", path+".bat")
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
			return candidate, true
		}
	}
	return "", false
}

func lookPath(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	for _, dir := range installDirs() {
		if path, ok := executable(filepath.Join(dir, name)); ok {
			return path, true
		}
	}
	return "", false
}
