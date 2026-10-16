// Package clipboard copies text to the system clipboard from a terminal app.
package clipboard

import (
	"encoding/base64"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Write copies text two ways: OSC 52, which asks the terminal to set the
// clipboard (works over SSH and in Ghostty), and the OS clipboard command
// when one is available (works when a multiplexer drops OSC 52).
func Write(text string) error {
	osc := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
	os.Stdout.WriteString(osc)
	return native(text)
}

func native(text string) error {
	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"pbcopy"}}
	case "windows":
		candidates = [][]string{{"clip"}}
	default:
		candidates = [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	}
	var err error = exec.ErrNotFound
	for _, c := range candidates {
		if _, lookErr := exec.LookPath(c[0]); lookErr != nil {
			continue
		}
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err = cmd.Run(); err == nil {
			return nil
		}
	}
	return err
}
