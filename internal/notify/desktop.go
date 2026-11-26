package notify

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Desktop sends local desktop notifications: notify-send on Linux,
// osascript on macOS. On WSL, other systems, or when the tool is missing it
// prints to Out instead. Nothing leaves the machine.
type Desktop struct {
	GOOS     string
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
	LookPath func(string) (string, error)
	Exec     func(name string, args ...string) error
	Out      io.Writer
}

// NewDesktop wires Desktop to the real system.
func NewDesktop(out io.Writer) *Desktop {
	return &Desktop{
		GOOS:     runtime.GOOS,
		Getenv:   os.Getenv,
		ReadFile: os.ReadFile,
		LookPath: exec.LookPath,
		Exec:     func(name string, args ...string) error { return exec.Command(name, args...).Run() },
		Out:      out,
	}
}

// isWSL reports whether we run under Windows Subsystem for Linux, where
// notify-send has no desktop to talk to.
func (d *Desktop) isWSL() bool {
	if d.Getenv("WSL_DISTRO_NAME") != "" || d.Getenv("WSL_INTEROP") != "" {
		return true
	}
	data, err := d.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// Method names the delivery mechanism: notify-send, osascript or stdout.
func (d *Desktop) Method() string {
	switch d.GOOS {
	case "linux":
		if d.isWSL() {
			return "stdout"
		}
		if _, err := d.LookPath("notify-send"); err == nil {
			return "notify-send"
		}
	case "darwin":
		if _, err := d.LookPath("osascript"); err == nil {
			return "osascript"
		}
	}
	return "stdout"
}

// Send delivers one notification. Title and body travel as separate argv
// entries, never through a shell or interpolated script.
func (d *Desktop) Send(title, body string) error {
	switch d.Method() {
	case "notify-send":
		return d.Exec("notify-send", "--app-name=gotodo", "--", title, body)
	case "osascript":
		return d.Exec("osascript",
			"-e", "on run argv",
			"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
			"-e", "end run",
			title, body)
	}
	_, err := fmt.Fprintf(d.Out, "%s: %s\n", title, body)
	return err
}
