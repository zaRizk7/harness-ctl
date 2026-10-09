// Package editor owns private temporary requests and native editor invocation.
package editor

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// platform isolates request publication failures from terminal interaction.
var platform = struct {
	create func(string, string) (*os.File, error)
	write  func(*os.File, []byte) (int, error)
	close  func(*os.File) error
}{os.CreateTemp, (*os.File).Write, (*os.File).Close}

// Open creates a mode-0600 request containing data and returns its editor command.
// finish receives the private path and process error before the file is removed.
// Publication failures invoke finish with no path. VISUAL takes precedence over
// EDITOR and the native default is vi. Editor configuration is user-owned shell code.
func Open(pattern string, data []byte, finish func(string, error) tea.Msg) tea.Cmd {
	file, err := platform.create("", pattern)
	if err != nil {
		return func() tea.Msg { return finish("", err) }
	}
	path := file.Name()
	if _, err = platform.write(file, data); err != nil {
		platform.close(file)
		os.Remove(path)
		return func() tea.Msg { return finish("", err) }
	}
	if err = platform.close(file); err != nil {
		os.Remove(path)
		return func() tea.Msg { return finish("", err) }
	}
	command := os.Getenv("VISUAL")
	if command == "" {
		command = os.Getenv("EDITOR")
	}
	if command == "" {
		command = "vi"
	}
	quoted := "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
	return tea.ExecProcess(exec.Command("/bin/sh", "-c", command+" "+quoted), func(err error) tea.Msg { defer os.Remove(path); return finish(path, err) })
}
