package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Server describes a language server servef can run. Servers are data and
// serve only document families servef already navigates; adding one is a
// deliberate registry change (see AGENTS.md).
type Server struct {
	// Name is the server's binary and display name.
	Name string
	Args []string
	// Language labels the families it serves, e.g. "Go".
	Language string
	// Families maps document family IDs to LSP language IDs.
	Families map[string]string
	Install  []Install
	Docs     string
}

// Install is one way to install a server.
type Install struct {
	Label   string `json:"label"`
	Command string `json:"command"`
}

// Builtins lists the supported language servers.
func Builtins() []Server {
	return []Server{
		{
			Name:     "gopls",
			Language: "Go",
			Families: map[string]string{"go": "go"},
			Install: []Install{
				{Label: "Go", Command: "go install golang.org/x/tools/gopls@latest"},
				{Label: "Homebrew", Command: "brew install gopls"},
			},
			Docs: "https://go.dev/gopls/",
		},
		{
			Name:     "typescript-language-server",
			Args:     []string{"--stdio"},
			Language: "JavaScript and TypeScript",
			Families: map[string]string{
				"javascript": "javascript",
				"jsx":        "javascriptreact",
				"typescript": "typescript",
				"tsx":        "typescriptreact",
			},
			Install: []Install{
				{Label: "npm", Command: "npm install -g typescript-language-server typescript"},
				{Label: "Homebrew", Command: "brew install typescript-language-server"},
			},
			Docs: "https://github.com/typescript-language-server/typescript-language-server",
		},
	}
}

// installDirs are where installers put binaries when servef was started
// without them on PATH, e.g. from a desktop launcher.
func installDirs() []string {
	var dirs []string
	if bin := os.Getenv("GOBIN"); bin != "" {
		dirs = append(dirs, bin)
	}
	if path := os.Getenv("GOPATH"); path != "" {
		dirs = append(dirs, filepath.Join(filepath.SplitList(path)[0], "bin"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, "go", "bin"),
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".npm-global", "bin"),
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".volta", "bin"),
		)
	}
	for _, name := range []string{"PNPM_HOME", "NVM_BIN"} {
		if dir := os.Getenv(name); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return append(dirs, "/opt/homebrew/bin", "/usr/local/bin")
}

// lookPath finds a server binary on PATH, then in installDirs.
func lookPath(name string) (string, bool) {
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	for _, dir := range installDirs() {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, true
		}
	}
	return "", false
}
