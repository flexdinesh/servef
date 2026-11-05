//go:build e2e

// Package e2e launches the built app on a display and drives its page over
// mygo's automation protocol (decision 0094). Run it under a display and a
// session bus, with the real servef binary:
//
//	xvfb-run -a dbus-run-session -- sh apps/desktop/scripts/e2e.sh
//
// It writes screenshots and report.json to SERVEF_DESKTOP_E2E_OUT. Resource
// scenarios additionally write RSS files when SERVEF_DESKTOP_EXTENDED=1.
package e2e

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	pageTimeout = 45 * time.Second
	callTimeout = 8 * time.Second
)

// session is a launch of the app and a connection to its automation socket.
type session struct {
	t      *testing.T
	cmd    *exec.Cmd
	socket string
	conn   net.Conn
	reader *bufio.Reader
	nextID int
	// target is the window evals run in; 0 is the focused window.
	target int
}

// connectionLink finds the link `servef connect` prints.
var connectionLink = regexp.MustCompile(`desktop: servef://(r-[0-9a-f]{12})/`)

func proxyOrigin(host string) string { return "servef-app://" + host }

type windowListing struct {
	Windows []struct {
		ID  int    `json:"id"`
		URL string `json:"url"`
	} `json:"windows"`
}

func (s *session) listWindows() windowListing {
	s.t.Helper()
	var listing windowListing
	if err := s.call("windows", nil, &listing); err != nil {
		s.t.Fatal(err)
	}
	return listing
}

func (s *session) windowCount() int {
	s.t.Helper()
	return len(s.listWindows().Windows)
}

// windowOn returns the window showing origin, or 0.
func (s *session) windowOn(origin string) int {
	s.t.Helper()
	for _, window := range s.listWindows().Windows {
		if strings.HasPrefix(window.URL, origin) {
			return window.ID
		}
	}
	return 0
}

// waitWindow waits for a window showing origin.
func (s *session) waitWindow(origin string) int {
	s.t.Helper()
	deadline := time.Now().Add(pageTimeout)
	for {
		if id := s.windowOn(origin); id != 0 {
			return id
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("no window on %s: %+v", origin, s.listWindows())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type world struct {
	t       *testing.T
	bin     string
	env     []string
	sock    string
	daemon  string
	project string
	out     string
	notices string
	report  map[string]any
}

func newWorld(t *testing.T) (*world, string) {
	t.Helper()
	binary := os.Getenv("SERVEF_DESKTOP_BINARY")
	if binary == "" {
		t.Fatal("SERVEF_DESKTOP_BINARY must name the servef binary")
	}
	out := os.Getenv("SERVEF_DESKTOP_E2E_OUT")
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	// Socket paths are short: t.TempDir under a long test name is too long.
	base, err := os.MkdirTemp("", "snv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	w := &world{t: t, out: out, report: map[string]any{}, sock: filepath.Join(base, "a.sock"), daemon: filepath.Join(base, "d.sock")}
	w.project = filepath.Join(base, "project")
	writeFixture(t, w.project)
	w.notices = filepath.Join(base, "notices.jsonl")
	// Dagger may supply the app built from the same immutable source snapshot.
	w.bin = os.Getenv("SERVEF_DESKTOP_APP_BINARY")
	if w.bin == "" {
		w.bin = filepath.Join(base, "servef-desktop")
		build := exec.Command("go", "build", "-tags", "e2e", "-o", w.bin, ".")
		build.Dir = ".."
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("building the app: %v\n%s", err, output)
		}
	}
	home := filepath.Join(base, "home")
	w.env = append(os.Environ(),
		"HOME="+home, "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_CONFIG_HOME="+filepath.Join(home, "config"), "XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"XDG_RUNTIME_DIR="+filepath.Join(base, "run"),
		"SERVEF_DESKTOP_SOCKET="+w.daemon, "SERVEF_DESKTOP_ROOTS_FILE="+filepath.Join(base, "roots.json"),
		"MYGO_AUTOMATION="+w.sock, "MYGO_ENV=production", "SERVEF_E2E_NOTICES="+w.notices,
	)
	for _, dir := range []string{home, filepath.Join(base, "run")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		stop := exec.Command(binary, "daemon", "stop", "--socket", w.daemon)
		stop.Env = w.env
		_ = stop.Run()
		data, _ := json.MarshalIndent(w.report, "", "  ")
		_ = os.WriteFile(filepath.Join(out, "report.json"), data, 0o644)
	})
	return w, base
}

func TestWebViewApp(t *testing.T) {
	w, base := newWorld(t)
	binary := os.Getenv("SERVEF_DESKTOP_BINARY")
	home := filepath.Join(base, "home")

	app := w.launch(w.project)
	app.waitEval("the project's tree", treeShows("README.md"))
	w.report["userAgent"] = app.eval(`return navigator.userAgent`)
	w.report["origin"] = app.eval(`return location.origin`)
	app.capture("workspace.png")

	// Failures in one part leave the others to run: each part reports through
	// its own t.
	parent := t
	run := func(name string, part func(t *testing.T)) {
		parent.Run(name, func(t *testing.T) {
			app.t = t
			defer func() { app.t = parent }()
			part(t)
		})
	}

	run("secure context", func(t *testing.T) {
		if got := app.eval(`return window.isSecureContext`); got != true {
			t.Errorf("isSecureContext = %v", got)
		}
		if got := app.eval(`return typeof crypto.subtle`); got != "object" {
			t.Errorf("crypto.subtle = %v", got)
		}
		app.eval(`localStorage.setItem("e2e-persist", "kept"); return true`)
		w.report["clipboardWrite"] = app.eval(`try { await navigator.clipboard.writeText("e2e"); return "ok" } catch (error) { return String(error) }`)
	})

	run("refusals", func(t *testing.T) {
		// "//x" alone would name another host, which the page's CSP refuses.
		for _, name := range []string{"/_daemon/status", "/%5Fdaemon/status", "//_daemon/status", "/_servef/status", "/x/%2e%2e/_daemon/status", "/_daemon/..%2fstatus", "/_daemon%2f..%2fstatus", "/_servef/..%2fstatus"} {
			got := app.eval(fmt.Sprintf(`return (await fetch(location.origin + %q)).status`, name))
			if got != float64(403) {
				t.Errorf("fetch %s = %v, want 403", name, got)
			}
		}
	})

	run("post bodies arrive", func(t *testing.T) {
		got := app.eval(`const r = await fetch("/api/walkthroughs", {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({unknownField: 1})}); return [r.status, await r.text()]`)
		values, _ := got.([]any)
		if len(values) != 2 || values[0] != float64(400) || !strings.Contains(fmt.Sprint(values[1]), "unknownField") {
			t.Errorf("POST answered %v: the server should reject the field it read", got)
		}
	})

	run("bytes", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(w.project, "pic.png"))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		want := hex.EncodeToString(sum[:])
		project := strconv.Quote(w.project)
		for _, endpoint := range []string{"/api/image", "/api/raw"} {
			got := app.eval(`const r = await fetch(` + strconv.Quote(endpoint) + ` + "?project=" + encodeURIComponent(` + project + `) + "&path=pic.png");
				const bytes = await r.arrayBuffer();
				const hash = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes)), (b) => b.toString(16).padStart(2, "0")).join("");
				return [r.status, r.headers.get("content-type"), bytes.byteLength, hash]`)
			values, _ := got.([]any)
			if len(values) != 4 || values[0] != float64(200) || values[1] != "image/png" || values[2] != float64(len(data)) || values[3] != want {
				t.Errorf("%s = %v, want %d bytes with sha256 %s", endpoint, got, len(data), want)
			}
		}
		// The server has no range support and the web asks for none: a Range
		// header passes through and the whole image comes back unchanged.
		ranged := app.eval(`const r = await fetch("/api/raw?project=" + encodeURIComponent(` + project + `) + "&path=pic.png", {headers: {Range: "bytes=0-3"}}); return [r.status, (await r.arrayBuffer()).byteLength]`)
		w.report["rawWithRange"] = ranged
		if values, _ := ranged.([]any); len(values) != 2 || values[1] != float64(len(data)) {
			t.Errorf("/api/raw with Range = %v", ranged)
		}
		// An <img> of the page's own origin, with what CSP says about it.
		image := app.eval(`const violations = [];
			document.addEventListener("securitypolicyviolation", (event) => violations.push(event.effectiveDirective + " " + event.blockedURI));
			const image = new Image();
			image.src = "/api/raw?project=" + encodeURIComponent(` + project + `) + "&path=pic.png";
			await new Promise((resolve) => { image.onload = resolve; image.onerror = resolve });
			await new Promise((resolve) => setTimeout(resolve, 100));
			return [image.naturalWidth, violations]`)
		w.report["sameOriginImage"] = image
		if values, _ := image.([]any); len(values) != 2 || values[0] != float64(4) {
			t.Errorf("img from /api/raw = %v, want naturalWidth 4", image)
		}
	})

	run("event streams", func(t *testing.T) {
		project := strconv.Quote(w.project)
		app.eval(`window.__stream = {events: [], source: new EventSource("/api/events?project=" + encodeURIComponent(` + project + `))};
			for (const name of ["status", "head", "change"]) window.__stream.source.addEventListener(name, (event) => window.__stream.events.push([name, Date.now(), event.data]));
			return true`)
		app.waitEval("the stream's first events", `return window.__stream.events.some((event) => event[0] === "head")`)
		if err := os.WriteFile(filepath.Join(w.project, "live.md"), []byte("# one\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		app.waitEval("a change over the stream", `return window.__stream.events.some((event) => event[0] === "change" && event[2].includes("live.md"))`)
		first := app.eval(`return window.__stream.events.filter((event) => event[0] === "change").length`)
		// A second change while the stream stays open: nothing buffers.
		if err := os.WriteFile(filepath.Join(w.project, "live2.md"), []byte("# two\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		app.waitEval("a second change over the same stream", `return window.__stream.events.some((event) => event[0] === "change" && event[2].includes("live2.md"))`)
		w.report["streamEvents"] = app.eval(`return window.__stream.events.map((event) => [event[0], event[2].slice(0, 60)])`)
		w.report["changesBeforeSecond"] = first
		w.report["streamState"] = app.eval(`return window.__stream.source.readyState`)

		// Closing streams ends their upstream requests within a keepalive tick.
		before := countSocketPeers(t, w.daemon)
		app.eval(`window.__streams = [1, 2, 3].map(() => new EventSource("/api/events?project=" + encodeURIComponent(` + project + `))); return true`)
		app.waitEval("three streams open", `return window.__streams.every((source) => source.readyState === EventSource.OPEN)`)
		time.Sleep(500 * time.Millisecond)
		open := countSocketPeers(t, w.daemon)
		app.eval(`window.__streams.forEach((source) => source.close()); window.__stream.source.close(); return true`)
		closed := time.Now()
		var after int
		for time.Since(closed) < 15*time.Second {
			after = countSocketPeers(t, w.daemon)
			if after <= open-3 {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		w.report["streamCancel"] = map[string]any{"before": before, "open": open, "after": after, "seconds": time.Since(closed).Seconds()}
		if after > open-3 {
			t.Errorf("daemon connections %d with streams, %d after closing: the upstream requests did not end", open, after)
		}
	})

	run("document, highlighting, diagram, links", func(t *testing.T) {
		w.exerciseDocuments(app)
	})

	run("live update reaches the page", func(t *testing.T) {
		app.eval(`location.href = "/view?project=" + encodeURIComponent(` + strconv.Quote(w.project) + `) + "&path=README.md"; return true`)
		app.waitEval("the document", `return document.body.innerText.includes("A paragraph.")`)
		file, err := os.OpenFile(filepath.Join(w.project, "README.md"), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = file.WriteString("\nLive marker from the e2e test.\n")
		_ = file.Close()
		app.waitEval("the changed file in the page", `return document.body.innerText.includes("Live marker from the e2e test.")`)
	})

	run("bridge", func(t *testing.T) {
		if got := app.eval(`return window.servefDesktop.version`); got != float64(2) {
			t.Errorf("bridge version = %v", got)
		}
		refused := func(script string) {
			t.Helper()
			got := app.eval(`try { return ["resolved", await ` + script + `] } catch (error) { return ["rejected", String(error)] }`)
			if values, _ := got.([]any); len(values) != 2 || values[0] != "rejected" {
				t.Errorf("%s = %v, want a refusal", script, got)
			}
		}
		refused(`window.servefDesktop.revealPath("relative", "a")`)
		refused(`window.servefDesktop.revealPath(` + strconv.Quote(w.project) + `, "../escape")`)
		refused(`window.servefDesktop.showContextMenu([], 1, 1)`)
		refused(`window.servefDesktop.showContextMenu([{id: "a b", label: "A"}], 1, 1)`)
		refused(`window.servefDesktop.showContextMenu([{id: "a", label: "A"}], -1, 1)`)
		if got := app.eval(`return await window.servefDesktop.revealPath(` + strconv.Quote(w.project) + `, "README.md")`); got != true {
			t.Errorf("revealPath of a served file = %v", got)
		}
		outside := filepath.Join(w.project, "escape")
		if err := os.Symlink("/etc/hostname", outside); err != nil {
			t.Fatal(err)
		}
		if got := app.eval(`return await window.servefDesktop.revealPath(` + strconv.Quote(w.project) + `, "escape")`); got != false {
			t.Errorf("revealPath through a symlink out of the project = %v", got)
		}
		app.eval(`window.__chosen = "pending"; void window.servefDesktop.showContextMenu([{id: "first", label: "First"}, {separator: true}, {id: "second", label: "Second"}], 40, 40).then((id) => { window.__chosen = id }); return true`)
		time.Sleep(time.Second)
		app.capture("context-menu.png")
		key := exec.Command("xdotool", "key", "--delay", "100", "Down", "Return")
		if output, err := key.CombinedOutput(); err != nil {
			t.Fatalf("xdotool: %v\n%s", err, output)
		}
		app.waitEval("the chosen menu item", `return window.__chosen !== "pending"`)
		w.report["contextMenuChosen"] = app.eval(`return window.__chosen`)
		if got := app.eval(`return window.__chosen`); got != "first" {
			t.Errorf("chosen = %v, want first", got)
		}
	})

	readme := `"/view?project=" + encodeURIComponent(` + strconv.Quote(w.project) + `) + "&path=README.md"`
	run("navigation stays in the app", func(t *testing.T) {
		app.eval(`location.href = ` + readme + `; return true`)
		app.waitEval("the document", `return document.body.innerText.includes("A paragraph.")`)
		before := app.eval(`return location.href`)
		for _, target := range []string{"https://example.invalid/out", "file:///etc/hostname"} {
			app.eval(`location.href = ` + strconv.Quote(target) + `; return true`)
			time.Sleep(1500 * time.Millisecond)
			if got := app.eval(`return location.href`); got != before {
				t.Errorf("navigating to %s left the page at %v", target, got)
			}
		}
		// The app's own scheme routes to the window, as a link does.
		app.eval(`location.href = "servef://local/view?project=" + encodeURIComponent(` + strconv.Quote(w.project) + `) + "&path=main.go"; return true`)
		app.waitEval("the link's document", `return location.origin + location.pathname === "servef-app://local/view" && location.search.includes("path=main.go")`)
	})

	run("bridge refuses other pages of the origin", func(t *testing.T) {
		app.eval(`location.href = "/api/projects"; return true`)
		app.waitEval("the JSON page", `return location.pathname === "/api/projects" && document.readyState === "complete" && window.servefDesktop !== undefined`)
		got := app.eval(`try { return ["resolved", await window.servefDesktop.revealPath(` + strconv.Quote(w.project) + `, "README.md")] } catch (error) { return ["rejected", String(error)] }`)
		if values, _ := got.([]any); len(values) != 2 || values[0] != "rejected" {
			t.Errorf("revealPath from /api/projects = %v, want a refusal", got)
		}
		w.report["bridgeFromOtherPage"] = got
	})

	run("bridge refuses a raw file", func(t *testing.T) {
		app.eval(`location.href = "/api/raw?project=" + encodeURIComponent(` + strconv.Quote(w.project) + `) + "&path=pic.png"; return true`)
		// The raw page's sandbox may stop scripts: nothing to wait on there.
		time.Sleep(2 * time.Second)
		got, err := app.evalErr(`try { return ["resolved", await window.servefDesktop.revealPath(` + strconv.Quote(w.project) + `, "README.md")] } catch (error) { return ["rejected", String(error)] }`)
		w.report["bridgeFromRaw"] = fmt.Sprint(got, " ", err)
		if values, _ := got.([]any); len(values) == 2 && values[0] == "resolved" {
			t.Errorf("revealPath from /api/raw = %v, want a refusal", got)
		}
		// Back through a link, which needs no script in the page.
		second := exec.Command(w.bin, "servef://local/view?project="+urlEscape(w.project)+"&path=README.md")
		second.Env = w.env
		if output, err := second.CombinedOutput(); err != nil {
			t.Fatalf("second instance: %v\n%s", err, output)
		}
		app.waitEval("the page after the link", treeShows("README.md"))
	})

	run("opening a folder after the daemon idled out", func(t *testing.T) {
		// A page without event streams: nothing else ensures the daemon.
		app.eval(`location.href = "/api/projects"; return true`)
		app.waitEval("the JSON page", `return location.pathname === "/api/projects" && document.readyState === "complete"`)
		stop := exec.Command(binary, "daemon", "stop", "--socket", w.daemon)
		stop.Env = w.env
		if output, err := stop.CombinedOutput(); err != nil {
			t.Fatalf("daemon stop: %v\n%s", err, output)
		}
		second := exec.Command(w.bin, w.project)
		second.Env = w.env
		if output, err := second.CombinedOutput(); err != nil {
			t.Errorf("second instance: %v\n%s", err, output)
		}
		// The landing page goes on to the project's last tab.
		app.waitEval("the folder's project", `return location.pathname !== "/api/projects" && location.search.includes("project=")`)
	})

	run("a crashed page reloads once, then says so", func(t *testing.T) {
		app.waitEval("the page", treeShows("README.md"))
		w.killWebProcess(t, app.cmd.Process.Pid)
		app.waitEval("the page after one crash", treeShows("README.md"))
		w.killWebProcess(t, app.cmd.Process.Pid)
		app.waitEval("the crash page", `return document.body.innerText.includes("crashed")`)
		app.capture("crashed.png")
		app.eval(`document.querySelector("a").click(); return true`)
		app.waitEval("the page after Try again", treeShows("README.md"))
	})

	app.stop()

	t.Run("state survives a restart", func(t *testing.T) {
		w.t = t
		// The install scripts replace only servef-desktop/app and keep the rest,
		// so the page's state must be beside it, where they expect it.
		stored := storageFiles(t, home)
		w.report["storageFiles"] = stored
		if len(stored) == 0 {
			t.Error("no localStorage files found")
		}
		for _, file := range stored {
			if !strings.HasPrefix(file, filepath.Join("data", "servef-desktop", "localstorage")+string(filepath.Separator)) {
				t.Errorf("localStorage at %s", file)
			}
		}
		app := w.launch(w.project)
		defer app.stop()
		app.waitEval("the page", treeShows("README.md"))
		if got := app.eval(`return localStorage.getItem("e2e-persist")`); got != "kept" {
			t.Errorf("localStorage after restart = %v", got)
		}
		app.waitEval("the restored tabs", `return Array.from(document.querySelectorAll("[role=tab]")).some((tab) => tab.textContent.includes("main.go"))`)
		app.capture("restored.png")
	})

	// servefGate runs the real binary, failing or slow at `daemon ensure` while
	// the files beside it say so.
	gate := filepath.Join(base, "servef-gate")
	gateClosed, gateSlow := filepath.Join(base, "gate-closed"), filepath.Join(base, "gate-slow")
	script := "#!/bin/sh\nif [ \"$1 $2\" = \"daemon ensure\" ]; then\n  if [ -e " + gateClosed + " ]; then echo 'gate closed' >&2; exit 1; fi\n  if [ -e " + gateSlow + " ]; then sleep 4; fi\nfi\nexec " + binary + " \"$@\"\n"
	if err := os.WriteFile(gate, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	isolate := func(name string) {
		home := filepath.Join(base, name)
		if err := os.MkdirAll(home, 0o700); err != nil {
			t.Fatal(err)
		}
		w.daemon = filepath.Join(base, name+".sock")
		w.env = append(w.env, "SERVEF_DESKTOP_BINARY="+gate, "HOME="+home, "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_CONFIG_HOME="+filepath.Join(home, "config"), "XDG_CACHE_HOME="+filepath.Join(home, "cache"),
			"SERVEF_DESKTOP_SOCKET="+w.daemon, "SERVEF_DESKTOP_ROOTS_FILE="+filepath.Join(base, name+"-roots.json"))
		t.Cleanup(func() {
			stop := exec.Command(binary, "daemon", "stop", "--socket", filepath.Join(base, name+".sock"))
			stop.Env = w.env
			_ = stop.Run()
		})
	}

	// useHandler makes w a home where the built app handles servef:// links
	// (real xdg-mime and xdg-open), with the CLI's default socket and roots.
	useHandler := func(t *testing.T, name string) {
		t.Helper()
		home := filepath.Join(base, name)
		dataHome := filepath.Join(home, "data")
		if err := os.MkdirAll(filepath.Join(dataHome, "applications"), 0o700); err != nil {
			t.Fatal(err)
		}
		entry := "[Desktop Entry]\nType=Application\nName=servef\nExec=" + w.bin + " %U\nTerminal=false\nMimeType=x-scheme-handler/servef;\n"
		if err := os.WriteFile(filepath.Join(dataHome, "applications", "servef-desktop.desktop"), []byte(entry), 0o644); err != nil {
			t.Fatal(err)
		}
		// The CLI knows only the default socket and roots: so must the app here.
		run := filepath.Join(base, name+"-r")
		if err := os.MkdirAll(run, 0o700); err != nil {
			t.Fatal(err)
		}
		w.env = append(w.env, "SERVEF_DESKTOP_BINARY="+binary, "HOME="+home, "XDG_DATA_HOME="+dataHome, "XDG_CONFIG_HOME="+filepath.Join(home, "config"),
			"XDG_CACHE_HOME="+filepath.Join(home, "cache"), "XDG_RUNTIME_DIR="+run, "SERVEF_DESKTOP_SOCKET=", "SERVEF_DESKTOP_ROOTS_FILE=", "TMPDIR="+base,
			"SSH_CONNECTION=", "SSH_CLIENT=", "SSH_TTY=")
		t.Cleanup(func() {
			stop := exec.Command(binary, "daemon", "stop")
			stop.Env = w.env
			_ = stop.Run()
		})
		register := exec.Command("xdg-mime", "default", "servef-desktop.desktop", "x-scheme-handler/servef")
		register.Env = w.env
		if output, err := register.CombinedOutput(); err != nil {
			t.Fatalf("xdg-mime default: %v\n%s", err, output)
		}
		query := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/servef")
		query.Env = w.env
		if output, err := query.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "servef-desktop.desktop" {
			t.Fatalf("xdg-mime query = %q, %v", output, err)
		}
	}

	// The way users get here: `servef <dir>` asks the OS for the servef://
	// handler and the running app takes the link.
	t.Run("servef opens a folder through the servef handler", func(t *testing.T) {
		w.t = t
		useHandler(t, "handler")
		app := w.launch()
		defer app.stop()
		other := filepath.Join(base, "handler project")
		writeFixture(t, other)
		cli := exec.Command(binary, other)
		cli.Env = w.env
		output, err := cli.CombinedOutput()
		w.report["cliOpen"] = string(output)
		if err != nil {
			t.Fatalf("servef %s: %v\n%s", other, err, output)
		}
		if !strings.Contains(string(output), "opened "+other+" in servef") || strings.Contains(string(output), "browser mode") {
			t.Errorf("servef printed %q", output)
		}
		app.waitEval("the folder's project in the window", `return location.origin === "servef-app://local" && new URLSearchParams(location.search).get("project") === `+strconv.Quote(other))
		app.waitEval("the project's tree", treeShows("README.md"))
		app.capture("cli-open.png")
	})

	t.Run("a first launch with no folder offers to open one", func(t *testing.T) {
		w.t = t
		isolate("empty")
		app := w.launch()
		defer app.stop()
		app.waitEval("the no folder state", `return document.body.innerText.includes("No folder is open.") && !document.body.innerText.includes("invalid page")`)
		app.waitEval("a status bar without an error", `return document.querySelector(".status-state").textContent.trim() === "Ready"`)
		app.capture("no-folder.png")
		app.eval(`[...document.querySelectorAll("button")].find((button) => button.textContent === "Open a folder").click(); return true`)
		app.waitEval("the picker's Open Folder", `return document.body.innerText.includes("No projects yet.")`)
		app.capture("no-folder-picker.png")
	})

	t.Run("close shortcut closes the active tab then the app", func(t *testing.T) {
		w.t = t
		isolate("close-tabs")
		app := w.launch(w.project)
		app.waitEval("Welcome", `return document.querySelector('[role=tab][aria-selected=true]')?.textContent === "Welcome"`)
		// Clicking the final tab's X keeps the empty workspace open.
		app.eval(`document.querySelector(".buffer-tab-close").click(); return true`)
		app.waitEval("the empty workspace", `return document.querySelectorAll('[role=tab]').length === 0`)
		for index, path := range []string{"README.md", "main.go"} {
			app.eval(`location.href = "/view?project=" + encodeURIComponent(` + strconv.Quote(w.project) + `) + "&path=" + ` + strconv.Quote(path) + `; return true`)
			app.waitEval("the open document tabs", fmt.Sprintf(`return document.querySelectorAll('[role=tab]').length === %d && document.querySelector('[role=tab][aria-selected=true]')?.textContent === %q`, index+1, path))
		}
		pressClose := func() {
			t.Helper()
			key := exec.Command("xdotool", "key", "--clearmodifiers", "ctrl+w")
			if output, err := key.CombinedOutput(); err != nil {
				t.Fatalf("xdotool: %v\n%s", err, output)
			}
		}
		waitForExit := func(app *session) {
			t.Helper()
			defer app.conn.Close()
			done := make(chan error, 1)
			go func() { done <- app.cmd.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("close shortcut exit: %v", err)
				}
			case <-time.After(pageTimeout):
				_ = app.cmd.Process.Kill()
				<-done
				t.Fatal("close shortcut did not quit the app")
			}
		}
		// Native shortcuts must work even while a text input owns focus.
		app.eval(`document.body.appendChild(document.createElement("input")).focus(); return true`)
		pressClose()
		app.waitEval("the previous tab selected", `return document.querySelectorAll('[role=tab]').length === 1 && document.querySelector('[role=tab][aria-selected=true]')?.textContent === "README.md"`)
		pressClose()
		waitForExit(app)

		app = w.launch(w.project)
		app.waitEval("Welcome without closed documents restored", `const tabs = document.querySelectorAll('[role=tab]'); return tabs.length === 1 && tabs[0].textContent === "Welcome"`)
		app.eval(`document.querySelector(".buffer-tab-close").click(); return true`)
		app.waitEval("the empty workspace", `return document.querySelectorAll('[role=tab]').length === 0`)
		pressClose()
		waitForExit(app)
	})

	t.Run("notices for a project on this computer", func(t *testing.T) {
		w.t = t
		useHandler(t, "notices")
		project := filepath.Join(base, "notices project")
		writeFixture(t, project)
		app := w.launch(project)
		defer app.stop()
		app.waitEval("the project's tree", treeShows("README.md"))
		w.exerciseNotices(app, project, "local", regexp.MustCompile(`^notices project · |^notices project$`), func() {
			draft, err := json.Marshal(map[string]any{"project": project, "title": "Notice tour", "steps": []map[string]any{{"title": "Start", "body": "Read this.", "file": map[string]any{"path": "README.md"}}}})
			if err != nil {
				t.Fatal(err)
			}
			got := app.eval(fmt.Sprintf(`const r = await fetch("/api/walkthroughs", {method: "POST", headers: {"Content-Type": "application/json"}, body: %s}); return r.ok`, strconv.Quote(string(draft))))
			if got != true {
				t.Fatalf("publishing a walkthrough answered %v", got)
			}
		})
	})

	// The whole remote flow (decision 0098): one machine runs --remote, this
	// computer runs the printed string, and the app shows it in a window of its
	// own. The two share nothing but that string.
	t.Run("servef connect opens a remote server in its own window", func(t *testing.T) {
		w.t = t
		useHandler(t, "connect")
		serverHome, serverRun := filepath.Join(base, "connect-server"), filepath.Join(base, "csrun")
		for _, dir := range []string{serverHome, serverRun} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		serverEnv := append(append([]string(nil), w.env...), "HOME="+serverHome, "XDG_DATA_HOME="+filepath.Join(serverHome, "data"), "XDG_CONFIG_HOME="+filepath.Join(serverHome, "config"),
			"XDG_CACHE_HOME="+filepath.Join(serverHome, "cache"), "XDG_RUNTIME_DIR="+serverRun)
		onServer := func(args ...string) (string, error) {
			command := exec.Command(binary, args...)
			command.Env = serverEnv
			output, err := command.CombinedOutput()
			return string(output), err
		}
		t.Cleanup(func() { _, _ = onServer("daemon", "stop") })
		served := filepath.Join(base, "connect project")
		writeFixture(t, served)
		printed, err := onServer(served, "--remote", "--host", "127.0.0.1", "--port", "0", "--remote-grace", "2s")
		if err != nil {
			t.Fatalf("servef --remote: %v\n%s", err, printed)
		}
		text := ""
		for line := range strings.Lines(printed) {
			if found, ok := strings.CutPrefix(strings.TrimSpace(line), "servef connect "); ok {
				text = found
			}
		}
		if text == "" {
			t.Fatalf("no connect string in %q", printed)
		}

		app := w.launch()
		defer app.stop()
		app.waitEval("the local window", `return location.origin === "servef-app://local"`)
		app.eval(`localStorage.setItem("connect-marker", "local"); return true`)
		local := app.windowOn(proxyOrigin("local"))

		connect := func() string {
			t.Helper()
			command := exec.Command(binary, "connect", text)
			command.Env = w.env
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("servef connect: %v\n%s", err, output)
			}
			return string(output)
		}
		output := connect()
		match := connectionLink.FindStringSubmatch(output)
		if match == nil || !strings.Contains(output, "connected to ") || strings.Contains(output, text[strings.LastIndex(text, "/")+1:]) {
			t.Fatalf("servef connect printed %q", output)
		}
		id := match[1]
		remote := app.waitWindow(proxyOrigin(id))
		if remote == local {
			t.Fatal("the connection opened in the local window")
		}
		app.target = remote
		app.waitEval("the remote project's tree", treeShows("README.md"))
		app.capture("remote-window.png")

		if got := app.eval(`return localStorage.getItem("connect-marker")`); got != nil {
			t.Errorf("the remote page reads the local page's storage: %v", got)
		}
		// Nothing of this machine: not on the bridge, and not by calling mygo.
		if got := app.eval(`const b = window.servefDesktop; return [b.version, typeof b.closeWindow, typeof b.showContextMenu, typeof b.showAppMenu, typeof b.openFolder, typeof b.revealPath]`); fmt.Sprint(got) != "[2 function function function undefined undefined]" {
			t.Errorf("remote bridge = %v", got)
		}
		for _, method := range []string{`"Bridge.RevealPath", ` + strconv.Quote(w.project) + `, "README.md"`, `"Bridge.OpenFolder"`} {
			if got := app.eval(`try { await window.mygo.call(` + method + `); return "resolved" } catch { return "refused" }`); got != "refused" {
				t.Errorf("a remote page called %s: %v", method, got)
			}
		}
		// No secret reaches the page, the control API stays out of reach, and
		// neither the daemon nor another origin answers it.
		if got := app.eval(`return [document.cookie, (await fetch("/_servef/remote")).status, (await fetch("/_daemon/status")).status]`); fmt.Sprint(got) != "[ 403 403]" {
			t.Errorf("cookie and control answers = %v", got)
		}
		if got := app.eval(`try { await fetch("servef-app://local/api/projects"); return "read" } catch { return "blocked" }`); got != "blocked" {
			t.Errorf("a remote page reached the local daemon: %v", got)
		}
		if strings.Contains(fmt.Sprint(app.eval(`return document.documentElement.outerHTML`)), text[strings.LastIndex(text, "/")+1:]) {
			t.Error("the credential is in the page")
		}

		// The server counts this window as a UI, though the page holds it open
		// through the app.
		clients := func() string {
			t.Helper()
			status, err := onServer("remote", "status", "--json")
			if err != nil {
				t.Fatalf("remote status: %v\n%s", err, status)
			}
			return status
		}
		deadline := time.Now().Add(pageTimeout)
		for !strings.Contains(clients(), `"clients":1`) {
			if time.Now().After(deadline) {
				t.Fatalf("the window never counted as a UI: %s", clients())
			}
			time.Sleep(200 * time.Millisecond)
		}
		// Past the grace, a window left open keeps the server.
		time.Sleep(3 * time.Second)
		if status := clients(); !strings.Contains(status, `"listening":true`) {
			t.Fatalf("the server closed under an open window: %s", status)
		}

		// Files changed on the server reach the window: first over the stream
		// already open, then after the server went away and came back, when the
		// page has to open a new one (the proxy answers errors while it is gone).
		writeServed := func(name string) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(served, name), []byte("# "+name+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		writeServed("live-remote.md")
		app.waitEval("a file created on the server", treeShows("live-remote.md"))
		if output, err := onServer("daemon", "stop"); err != nil {
			t.Fatalf("daemon stop: %v\n%s", err, output)
		}
		app.waitEval("the window noticing the server left", `return document.querySelector('.status-live[data-mode="off"]') !== null`)
		// Seconds of errors, so the browser has given up on the stream.
		time.Sleep(3 * time.Second)
		if output, err := onServer(served, "--remote", "--host", "127.0.0.1", "--remote-grace", "2s"); err != nil {
			t.Fatalf("servef --remote again: %v\n%s", err, output)
		}
		app.waitEval("the window following the server again", `return document.querySelector('.status-live[data-mode="watching"]') !== null`)
		writeServed("live-remote-again.md")
		app.waitEval("a file created after the server came back", treeShows("live-remote-again.md"))

		// Notices work as they do on this computer, over the notifier's own
		// streams, which came back with the server too (decision 0099). Each names
		// its connection, so a server cannot pass off as this computer.
		w.exerciseNotices(app, served, id, regexp.MustCompile(`^[^·]+ · connect project · |^[^·]+ · connect project$`), func() {
			draft := filepath.Join(base, "notice-tour.json")
			content := fmt.Sprintf(`{"project":%q,"title":"Notice tour","steps":[{"title":"Start","body":"Read this.","file":{"path":"README.md"}}]}`, served)
			if err := os.WriteFile(draft, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := onServer("walkthrough", "publish", draft, "--no-open"); err != nil {
				t.Fatalf("walkthrough publish: %v\n%s", err, output)
			}
		})

		// Connecting again brings the window forward and keeps what it shows.
		app.eval(`window.__stamp = 1; return true`)
		connect()
		time.Sleep(time.Second)
		if got := app.eval(`return window.__stamp`); got != float64(1) {
			t.Errorf("connecting again reloaded the window: %v", got)
		}
		if got := app.windowCount(); got != 2 {
			t.Errorf("%d windows after connecting again, want 2", got)
		}

		// The last UI closes: the server stops by itself, and the local window
		// is unaffected.
		app.eval(`window.servefDesktop.closeWindow(); return true`)
		app.target = local
		for app.windowCount() != 1 {
			if time.Now().After(deadline.Add(pageTimeout)) {
				t.Fatal("the remote window did not close")
			}
			time.Sleep(100 * time.Millisecond)
		}
		app.waitEval("the local window still answering", `return location.origin === "servef-app://local"`)
		stopBy := time.Now().Add(pageTimeout)
		for {
			if _, err := onServer("daemon", "status"); err != nil {
				break
			}
			if time.Now().After(stopBy) {
				status, _ := onServer("remote", "status", "--json")
				t.Fatalf("the server outlived its last UI: %s", status)
			}
			time.Sleep(200 * time.Millisecond)
		}

		// A link to a connection this computer does not have opens nothing.
		unknown := exec.Command(w.bin, "servef://r-000000000000/")
		unknown.Env = w.env
		if output, err := unknown.CombinedOutput(); err != nil {
			t.Fatalf("an unknown connection's link: %v\n%s", err, output)
		}
		time.Sleep(time.Second)
		if got := app.windowCount(); got != 1 {
			t.Errorf("%d windows after an unknown connection's link", got)
		}
	})

	t.Run("a link during a slow start wins", func(t *testing.T) {
		w.t = t
		isolate("slow")
		if err := os.WriteFile(gateSlow, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(gateSlow)
		second := make(chan error, 1)
		go func() {
			time.Sleep(1500 * time.Millisecond)
			link := exec.Command(w.bin, "servef://local/view?project="+urlEscape(w.project)+"&path=main.go")
			link.Env = w.env
			second <- link.Run()
		}()
		app := w.launch(w.project)
		defer app.stop()
		if err := <-second; err != nil {
			t.Errorf("second instance: %v", err)
		}
		app.waitEval("the link's document", `return location.pathname === "/view" && location.search.includes("path=main.go")`)
	})

	t.Run("Try again recovers from a daemon that would not start", func(t *testing.T) {
		w.t = t
		isolate("gate")
		if err := os.WriteFile(gateClosed, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		app := w.launch()
		defer app.stop()
		app.waitEval("the failure page", `return document.body.innerText.includes("can't reach its daemon") && document.body.innerText.includes("gate closed")`)
		app.capture("try-again.png")
		if err := os.Remove(gateClosed); err != nil {
			t.Fatal(err)
		}
		app.eval(`document.querySelector("a").click(); return true`)
		app.waitEval("the app after Try again", `const text = document.body.innerText; return text.includes("Files") && !text.includes("can't reach its daemon")`)
	})
}

// TestWebViewResources measures a document session and a fresh single-document
// profile without repeating the correctness suite.
func TestWebViewResources(t *testing.T) {
	if os.Getenv("SERVEF_DESKTOP_EXTENDED") != "1" {
		t.Skip("resource scenarios require SERVEF_DESKTOP_EXTENDED=1")
	}
	w, base := newWorld(t)
	t.Run("memory after the session", func(t *testing.T) {
		w.t = t
		app := w.launch(w.project)
		defer app.stop()
		app.waitEval("the project's tree", treeShows("README.md"))
		w.report["userAgent"] = app.eval(`return navigator.userAgent`)
		w.report["origin"] = app.eval(`return location.origin`)
		w.exerciseDocuments(app)
		time.Sleep(10 * time.Second)
		w.writeMemory(app.cmd.Process.Pid, "rss-session.txt")
	})

	// A fresh profile on the same daemon: Explorer and one Markdown document
	// with its highlighted fence and diagram, nothing else this page loaded.
	t.Run("memory with one document", func(t *testing.T) {
		w.t = t
		home := filepath.Join(base, "fresh")
		if err := os.MkdirAll(home, 0o700); err != nil {
			t.Fatal(err)
		}
		w.env = append(w.env, "HOME="+home, "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_CONFIG_HOME="+filepath.Join(home, "config"), "XDG_CACHE_HOME="+filepath.Join(home, "cache"))
		app := w.launch("servef://local/view?project=" + urlEscape(w.project) + "&path=README.md")
		defer app.stop()
		app.waitEval("the document's Preview button", `const preview = Array.from(document.querySelectorAll("button")).find((button) => button.textContent.trim() === "Preview"); preview?.click(); return preview !== undefined`)
		app.waitEval("a Mermaid diagram", `return document.querySelector(".mermaid-diagram svg.ondine") !== null`)
		w.report["memoryTabs"] = app.eval(`return document.querySelectorAll("[role=tab]").length`)
		time.Sleep(10 * time.Second)
		w.writeMemory(app.cmd.Process.Pid, "rss.txt")
		app.capture("one-document.png")
	})
}

// exerciseDocuments also supplies the representative resource workload.
func (w *world) exerciseDocuments(app *session) {
	app.t.Helper()
	// A second launch hands its link to the first instance.
	second := exec.Command(w.bin, "servef://local/view?project="+urlEscape(w.project)+"&path=main.go")
	second.Env = w.env
	if output, err := second.CombinedOutput(); err != nil {
		app.t.Errorf("second instance: %v\n%s", err, output)
	}
	app.waitEval("a highlighted keyword", `return Array.from(document.querySelectorAll(".hl-keyword")).some((span) => span.textContent.trim() === "package")`)
	app.capture("source.png")
	// A large source highlights in the worker, whose failure leaves plain text.
	app.eval(`location.href = "/view?project=" + encodeURIComponent(` + strconv.Quote(w.project) + `) + "&path=big.go"; return true`)
	app.waitEval("a large source highlighted in the worker", `return document.body.innerText.includes("Value0") && document.querySelector(".hl-keyword") !== null`)
	app.eval(`location.href = "/view?project=" + encodeURIComponent(` + strconv.Quote(w.project) + `) + "&path=README.md"; return true`)
	app.waitEval("the document's Preview button", `const preview = Array.from(document.querySelectorAll("button")).find((button) => button.textContent.trim() === "Preview"); preview?.click(); return preview !== undefined`)
	app.waitEval("a Mermaid diagram", `return document.querySelector(".mermaid-diagram svg.ondine") !== null`)
	app.waitEval("a Markdown image", `const image = document.querySelector("main img"); return image !== null && image.complete && image.naturalWidth === 4`)
	app.capture("diagram.png")
	app.eval(`location.href = "/view?project=" + encodeURIComponent(` + strconv.Quote(w.project) + `) + "&path=pic.png"; return true`)
	app.waitEval("the image viewer", `const image = document.querySelector("main img"); return image !== null && image.complete && image.naturalWidth === 4`)
	w.report["wasm"] = app.eval(`return performance.getEntriesByType("resource").map((entry) => entry.name).filter((name) => /wasm|worker/.test(name))`)
}

// killWebProcess crashes the page's web process, as a page that breaks it does.
func (w *world) killWebProcess(t *testing.T, appPid int) {
	t.Helper()
	data, err := exec.Command("pgrep", "-P", strconv.Itoa(appPid), "WebKitWebProces").Output()
	if err != nil {
		t.Fatalf("finding the web process: %v", err)
	}
	for _, pid := range strings.Fields(string(data)) {
		if output, err := exec.Command("kill", "-KILL", pid).CombinedOutput(); err != nil {
			t.Fatalf("kill %s: %v\n%s", pid, err, output)
		}
	}
}

// storageFiles lists, relative to home, where WebKit put localStorage.
func storageFiles(t *testing.T, home string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.Contains(strings.ToLower(path), "localstorage") {
			relative, _ := filepath.Rel(home, path)
			found = append(found, relative)
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
	return found
}

// treeShows is a script: the Explorer, inside a shadow root, has the row.
func treeShows(name string) string {
	return `const find = (root) => root.querySelector('[data-item-path=' + JSON.stringify(` + strconv.Quote(name) + `) + ']') !== null
		|| Array.from(root.querySelectorAll("*")).some((element) => element.shadowRoot !== null && find(element.shadowRoot));
	return find(document)`
}

func urlEscape(value string) string {
	var out strings.Builder
	for _, b := range []byte(value) {
		if b == '/' || b == '-' || b == '_' || b == '.' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
			out.WriteByte(b)
		} else {
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}

func (w *world) launch(args ...string) *session {
	w.t.Helper()
	cmd := exec.Command(w.bin, args...)
	cmd.Env = w.env
	log, err := os.OpenFile(filepath.Join(w.out, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		w.t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		w.t.Fatal(err)
	}
	s := &session{t: w.t, cmd: cmd, socket: w.sock}
	w.t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = log.Close()
	})
	deadline := time.Now().Add(pageTimeout)
	for {
		conn, err := net.Dial("unix", w.sock)
		if err == nil {
			s.conn, s.reader = conn, bufio.NewReader(conn)
			break
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("automation socket: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// The window appears once the daemon is ensured.
	for {
		var listing struct {
			Windows []struct {
				URL string `json:"url"`
			} `json:"windows"`
		}
		if err := s.call("windows", nil, &listing); err != nil {
			w.t.Fatal(err)
		}
		if len(listing.Windows) > 0 {
			break
		}
		if time.Now().After(deadline) {
			w.t.Fatal("the app opened no window")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return s
}

func (s *session) stop() {
	s.t.Helper()
	_ = s.conn.Close()
	_ = s.cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		s.t.Error("the app did not quit on SIGTERM")
		_ = s.cmd.Process.Kill()
		<-done
	}
}

func (s *session) call(method string, params map[string]any, result any) error {
	s.nextID++
	request, err := json.Marshal(map[string]any{"id": s.nextID, "method": method, "params": params})
	if err != nil {
		return err
	}
	if err := s.conn.SetDeadline(time.Now().Add(callTimeout)); err != nil {
		return err
	}
	if _, err := s.conn.Write(append(request, '\n')); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	line, err := s.reader.ReadBytes('\n')
	if err != nil {
		// A navigation can drop a script in flight, which is then never
		// answered, and answers come in order: a new connection goes on.
		if conn, dialErr := net.Dial("unix", s.socket); dialErr == nil {
			_ = s.conn.Close()
			s.conn, s.reader = conn, bufio.NewReader(conn)
		}
		return fmt.Errorf("%s: %w", method, err)
	}
	var answer struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &answer); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if answer.Error != nil {
		return fmt.Errorf("%s: %s: %s", method, answer.Error.Code, answer.Error.Message)
	}
	if result != nil {
		return json.Unmarshal(answer.Result, result)
	}
	return nil
}

// evalErr runs script as the body of an async function in the page.
func (s *session) evalErr(script string) (any, error) {
	var result struct {
		Value any `json:"value"`
	}
	params := map[string]any{"script": script}
	if s.target != 0 {
		params["window"] = s.target
	}
	err := s.call("eval", params, &result)
	return result.Value, err
}

func (s *session) eval(script string) any {
	s.t.Helper()
	value, err := s.evalErr(script)
	if err != nil {
		s.t.Errorf("eval %s: %v", script, err)
	}
	return value
}

// waitEval polls script until it returns true. Navigation drops an eval in
// flight, so errors are retried until the timeout.
func (s *session) waitEval(what, script string) {
	s.t.Helper()
	deadline := time.Now().Add(pageTimeout)
	var last any
	var lastErr error
	for time.Now().Before(deadline) {
		last, lastErr = s.evalErr(script)
		if lastErr == nil && last == true {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	body, _ := s.evalErr(`return [location.href, document.visibilityState, document.body.innerText.slice(0, 2000)]`)
	s.capture("failure-" + strings.ReplaceAll(what, " ", "-") + ".png")
	s.t.Fatalf("waiting for %s: last %v, %v; page: %v", what, last, lastErr, body)
}

func (s *session) capture(name string) {
	s.t.Helper()
	var result struct {
		PNG string `json:"png"`
	}
	if err := s.call("capture", nil, &result); err != nil {
		s.t.Errorf("capture %s: %v", name, err)
		return
	}
	data, err := base64.StdEncoding.DecodeString(result.PNG)
	if err != nil {
		s.t.Errorf("capture %s: %v", name, err)
		return
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("SERVEF_DESKTOP_E2E_OUT"), name), data, 0o644); err != nil {
		s.t.Errorf("capture %s: %v", name, err)
	}
}

// countSocketPeers counts open connections on the daemon's socket path: both
// the listener and accepted sockets name the path in /proc/net/unix.
func countSocketPeers(t *testing.T, socket string) int {
	t.Helper()
	data, err := os.ReadFile("/proc/net/unix")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasSuffix(line, " "+socket) {
			count++
		}
	}
	return count
}

// writeMemory records RSS and PSS of the app's process tree and, apart, of
// the daemon, into name in the output directory.
func (w *world) writeMemory(appPid int, name string) {
	children := map[int][]int{}
	names := map[int]string{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		w.t.Fatal(err)
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		text := string(data)
		open, closed := strings.IndexByte(text, '('), strings.LastIndexByte(text, ')')
		if open < 0 || closed < 0 {
			continue
		}
		names[pid] = text[open+1 : closed]
		fields := strings.Fields(text[closed+1:])
		if len(fields) > 1 {
			parent, _ := strconv.Atoi(fields[1])
			children[parent] = append(children[parent], pid)
		}
	}
	var report bytes.Buffer
	var totalRSS, totalPSS int
	var walk func(pid int)
	walk = func(pid int) {
		rss, pss := rollup(pid)
		totalRSS += rss
		totalPSS += pss
		fmt.Fprintf(&report, "app-tree %d %s rss_kib=%d pss_kib=%d\n", pid, names[pid], rss, pss)
		for _, child := range children[pid] {
			walk(child)
		}
	}
	walk(appPid)
	fmt.Fprintf(&report, "app-tree total rss_mib=%d pss_mib=%d\n", totalRSS/1024, totalPSS/1024)
	for pid, name := range names {
		if name == "servef" {
			rss, pss := rollup(pid)
			fmt.Fprintf(&report, "servef %d rss_mib=%d pss_mib=%d\n", pid, rss/1024, pss/1024)
		}
	}
	w.t.Log(report.String())
	if err := os.WriteFile(filepath.Join(w.out, name), report.Bytes(), 0o644); err != nil {
		w.t.Error(err)
	}
}

func rollup(pid int) (rss, pss int) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "smaps_rollup"))
	if err != nil {
		return 0, 0
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, _ := strconv.Atoi(fields[1])
		switch fields[0] {
		case "Rss:":
			rss = value
		case "Pss:":
			pss = value
		}
	}
	return rss, pss
}

func writeFixture(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"README.md": "# E2E fixture\n\nA paragraph.\n\n![pic](pic.png)\n\n```go\npackage main\n```\n\n```mermaid\nflowchart LR\n  A --> B\n```\n",
		"main.go":   "package main\n\n// A comment.\nfunc main() {}\n",
	}
	var big strings.Builder
	big.WriteString("package main\n\n")
	for i := range 4000 {
		fmt.Fprintf(&big, "const Value%d = %d // Line %d of a file past the main thread's highlight limit.\n", i, i, i)
	}
	files["big.go"] = big.String()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "pic.png"), paddedPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.name=e2e", "-c", "user.email=e2e@example.invalid", "commit", "-qm", "init"}} {
		git := exec.Command("git", args...)
		git.Dir = root
		if output, err := git.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
}

// paddedPNG is a valid 4x4 PNG of more than a MiB: an ancillary chunk spans
// mygo's 256 KiB write chunks and shows truncation or corruption in a hash.
func paddedPNG(t *testing.T) []byte {
	t.Helper()
	chunk := func(kind string, data []byte) []byte {
		var out bytes.Buffer
		_ = binary.Write(&out, binary.BigEndian, uint32(len(data)))
		out.WriteString(kind)
		out.Write(data)
		_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(kind), data...)))
		return out.Bytes()
	}
	var png bytes.Buffer
	png.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	png.Write(chunk("IHDR", []byte{0, 0, 0, 4, 0, 0, 0, 4, 8, 0, 0, 0, 0}))
	padding := make([]byte, 1<<20+7)
	for i := range padding {
		padding[i] = byte(i * 31)
	}
	png.Write(chunk("teXt", append([]byte("e2e\x00"), padding...)))
	// Stored deflate of 4 rows: a filter byte and 4 gray pixels each.
	raw := []byte{0, 1, 2, 3, 4, 0, 5, 6, 7, 8, 0, 9, 10, 11, 12, 0, 13, 14, 15, 16}
	stored := append([]byte{0x78, 0x01, 0x01, byte(len(raw)), 0, ^byte(len(raw)), 0xff}, raw...)
	stored = binary.BigEndian.AppendUint32(stored, adler32(raw))
	png.Write(chunk("IDAT", stored))
	png.Write(chunk("IEND", nil))
	return png.Bytes()
}

func adler32(data []byte) uint32 {
	a, b := uint32(1), uint32(0)
	for _, d := range data {
		a = (a + uint32(d)) % 65521
		b = (b + a) % 65521
	}
	return b<<16 | a
}
