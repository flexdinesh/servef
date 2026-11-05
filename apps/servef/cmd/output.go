package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

type startupInfo struct {
	Version      string
	Directories  []string
	URL          string
	Exposed      bool // bound to an address other machines may reach
	Links        bool // write URLs as OSC 8 hyperlinks
	ProjectCount int
	Discovery    time.Duration
}

func writeStartup(output io.Writer, info startupInfo) {
	output = terminalWriter(output)
	heading := color.New(color.Bold, color.FgHiMagenta)
	label := color.New(color.Faint)
	path := color.New(color.FgHiBlue)
	url := color.New(color.FgHiCyan, color.Underline)
	count := color.New(color.FgHiGreen)
	duration := color.New(color.FgHiYellow)

	_, _ = fmt.Fprint(output, "  ")
	_, _ = heading.Fprintln(output, info.Version)
	name := "serving directory:  "
	if len(info.Directories) > 1 {
		name = "serving directories:  "
	}
	for i, directory := range info.Directories {
		if i > 0 {
			name = strings.Repeat(" ", len(name))
		}
		writeStartupValue(output, label, path, name, directory)
	}
	writeStartupValue(output, label, url, "web:      ", hyperlink(info.URL, info.Links))
	if info.Exposed {
		_, _ = fmt.Fprint(output, "  ")
		_, _ = color.New(color.FgHiYellow).Fprintln(output, "no authentication: anyone who can reach this address can read the served files.")
	}
	_, _ = fmt.Fprintln(output)
	writeDiscovery(output, label, count, duration, info)
	_, _ = fmt.Fprint(output, "  ")
	_, _ = label.Fprintln(output, "ctrl-c to stop.")
}

// remoteStartup is what `servef --remote` prints.
type remoteStartup struct {
	Version     string
	Directories []string
	Connect     string // the connection string, ready for a shell
	Web         string // "" unless --web
	Links       bool   // write Web as an OSC 8 hyperlink
	FirstWait   time.Duration
	Grace       time.Duration
}

func writeRemoteStartup(output io.Writer, info remoteStartup) {
	output = terminalWriter(output)
	heading := color.New(color.Bold, color.FgHiMagenta)
	label := color.New(color.Faint)
	path := color.New(color.FgHiBlue)
	url := color.New(color.FgHiCyan, color.Underline)
	command := color.New(color.FgHiCyan)

	_, _ = fmt.Fprint(output, "  ")
	_, _ = heading.Fprintln(output, info.Version)
	name := "serving directory:  "
	if len(info.Directories) > 1 {
		name = "serving directories:  "
	}
	for i, directory := range info.Directories {
		if i > 0 {
			name = strings.Repeat(" ", len(name))
		}
		writeStartupValue(output, label, path, name, directory)
	}
	_, _ = fmt.Fprintln(output)
	_, _ = fmt.Fprint(output, "  ")
	_, _ = label.Fprintln(output, "on your computer, run:")
	_, _ = fmt.Fprint(output, "  ")
	_, _ = command.Fprintln(output, "servef connect "+info.Connect)
	if info.Web != "" {
		_, _ = fmt.Fprintln(output)
		writeStartupValue(output, label, url, "web:  ", hyperlink(info.Web, info.Links))
		_, _ = fmt.Fprint(output, "  ")
		_, _ = label.Fprintln(output, "browsers warn about its self-signed certificate; this link is a password.")
	}
	_, _ = fmt.Fprintln(output)
	_, _ = fmt.Fprint(output, "  ")
	_, _ = label.Fprintf(output, "stops %s after this if nothing connects, and %s after the last UI closes. `servef remote stop` stops it now.\n", minutesText(info.FirstWait), minutesText(info.Grace))
}

func writeDiscovery(output io.Writer, label, count, duration *color.Color, info startupInfo) {
	_, _ = fmt.Fprint(output, "  ")
	_, _ = label.Fprint(output, "projects discovered: ")
	_, _ = count.Fprint(output, info.ProjectCount)
	_, _ = fmt.Fprint(output, "  ")
	_, _ = duration.Fprintf(output, "(%dms)", info.Discovery.Milliseconds())
	_, _ = fmt.Fprintln(output)
}

func writeStartupValue(output io.Writer, label, value *color.Color, name string, content any) {
	_, _ = fmt.Fprint(output, "  ")
	_, _ = label.Fprint(output, name)
	_, _ = value.Fprintln(output, content)
}

func writePortBusy(output io.Writer, port int, hasNext bool) {
	output = terminalWriter(output)
	warning := color.New(color.FgHiYellow)
	_, _ = fmt.Fprint(output, "  ")
	if hasNext {
		_, _ = warning.Fprintf(output, "port %d is busy; trying %d.\n", port, port+1)
		return
	}
	_, _ = warning.Fprintf(output, "port %d is busy.\n", port)
}

func confirmStopServers(input io.Reader, output io.Writer, count int) bool {
	output = terminalWriter(output)
	prompt := color.New(color.Bold, color.FgHiYellow)
	_, _ = fmt.Fprint(output, "  ")
	_, _ = prompt.Fprintf(output, "all %d default ports are busy. terminate %d servef servers? [y/N] ", count, count)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && answer == "" {
		_, _ = fmt.Fprintln(output)
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func terminalWriter(output io.Writer) io.Writer {
	file, ok := output.(*os.File)
	if ok && file == os.Stdout {
		return color.Output
	}
	return output
}
