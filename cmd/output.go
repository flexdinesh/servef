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
	writeStartupValue(output, label, url, "url:  ", info.URL)
	_, _ = fmt.Fprintln(output)
	writeDiscovery(output, label, count, duration, info)
	_, _ = fmt.Fprint(output, "  ")
	_, _ = label.Fprintln(output, "ctrl-c to stop.")
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
