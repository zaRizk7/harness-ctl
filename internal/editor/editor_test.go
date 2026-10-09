package editor

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type model struct{ cmd tea.Cmd }

func (m model) Init() tea.Cmd                       { return tea.Sequence(m.cmd, tea.Quit) }
func (m model) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (model) View() tea.View                        { return tea.NewView("") }

func TestPrivateRequestPublicationFailuresCleanUp(t *testing.T) {
	for _, boundary := range []string{"create", "write", "close"} {
		t.Run(boundary, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			old := platform
			t.Cleanup(func() { platform = old })
			switch boundary {
			case "create":
				platform.create = func(string, string) (*os.File, error) { return nil, io.ErrClosedPipe }
			case "write":
				platform.write = func(*os.File, []byte) (int, error) { return 0, io.ErrClosedPipe }
			case "close":
				platform.close = func(f *os.File) error { _ = old.close(f); return io.ErrClosedPipe }
			}
			calls := 0
			cmd := Open("request-*", []byte("private"), func(path string, err error) tea.Msg {
				calls++
				if path != "" || !errors.Is(err, io.ErrClosedPipe) {
					t.Fatal(path, err)
				}
				return nil
			})
			cmd()
			files, err := os.ReadDir(dir)
			if err != nil || calls != 1 || len(files) != 0 {
				t.Fatal("failed request retained", files, err)
			}
		})
	}
}

func TestNativeEditorChoicesAndPrivateCallback(t *testing.T) {
	for _, choice := range []string{"visual", "editor", "default"} {
		t.Run(choice, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			t.Setenv("VISUAL", "")
			t.Setenv("EDITOR", "")
			// The default vi resolves to a fixture, avoiding an interactive editor in tests.
			if err := os.WriteFile(dir+"/vi", []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			if choice == "visual" {
				t.Setenv("VISUAL", "/usr/bin/true")
				t.Setenv("EDITOR", "/usr/bin/false")
			}
			if choice == "editor" {
				t.Setenv("EDITOR", "/usr/bin/true")
			}
			calls := 0
			cmd := Open("private-*.json", []byte("private"), func(path string, err error) tea.Msg {
				calls++
				data, e := os.ReadFile(path)
				info, statErr := os.Stat(path)
				if err != nil || e != nil || statErr != nil || string(data) != "private" || info.Mode().Perm() != 0600 {
					t.Fatal("private request contract", err, e, statErr)
				}
				return nil
			})
			if _, err := tea.NewProgram(model{cmd}, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard), tea.WithoutSignalHandler()).Run(); err != nil {
				t.Fatal(err)
			}
			files, err := os.ReadDir(dir)
			if err != nil || calls != 1 || len(files) != 1 {
				t.Fatal("request leaked", files, err)
			}
		})
	}
}
