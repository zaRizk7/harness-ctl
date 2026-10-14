package sdkreport

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestControlReportSkipsTranscriptsAndNeverSendsAPrompt(t *testing.T) {
	var out strings.Builder
	body := "{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"initialize\",\"response\":{\"claude_code_version\":\"2.1.296\"}}}\n{\"type\":\"control_response\",\"response\":{\"subtype\":\"success\",\"request_id\":\"usage\",\"response\":{\"rate_limits_available\":true,\"rate_limits\":{\"five_hour\":{\"utilization\":25}}}}}"
	data, err := readUsage(strings.NewReader(body), &out, "2.1.296", 4096, 8)
	if err != nil || !strings.Contains(string(data), `"utilization":25`) || !strings.Contains(out.String(), `"skip_behaviors":true`) || strings.Contains(out.String(), `"type":"user"`) {
		t.Fatal(string(data), out.String(), err)
	}
}

type failWriter struct{ after int }

func (w *failWriter) Write(p []byte) (int, error) {
	w.after--
	if w.after < 0 {
		return 0, errors.New("synthetic private diagnostic")
	}
	return len(p), nil
}

func TestUnsupportedControlRequestsAreBoundedAndRedacted(t *testing.T) {
	init := `{"type":"control_response","response":{"subtype":"success","request_id":"initialize","response":{"claude_code_version":"2.1.296"}}}` + "\n"
	for _, tc := range []struct {
		body     string
		bytes    int64
		messages int
	}{
		{"", 0, 1}, {"", 1, 0}, {"bad", 1000, 3}, {`{"type":"control_request"}`, 1000, 3}, {`{"type":"assistant"}`, 1000, 3},
		{`{"type":"control_response","response":{"subtype":"success","request_id":"usage"}}`, 1000, 3},
		{`{"type":"control_response","response":{"subtype":"error","request_id":"initialize","error":"private diagnostic"}}`, 1000, 3},
		{init + `{"type":"control_response","response":{"subtype":"success","request_id":"usage"}}`, 1000, 3},
		{init + `{"type":"control_response","response":{"subtype":"success","request_id":"usage","response":null}}`, 1000, 3},
		{`{"type":"keep_alive"}`, 1000, 1}, {init, 8, 3},
	} {
		if _, err := readUsage(strings.NewReader(tc.body), io.Discard, "2.1.296", tc.bytes, tc.messages); err == nil || strings.Contains(err.Error(), "private diagnostic") {
			t.Fatal(tc, err)
		}
	}
	for _, after := range []int{0, 1} {
		if _, err := readUsage(strings.NewReader(init), &failWriter{after: after}, "2.1.296", 4096, 3); err == nil {
			t.Fatal(after)
		}
	}
	var out strings.Builder
	if _, err := readUsage(strings.NewReader("{\"type\":\"keep_alive\"}\n"+init+`{"type":"control_response","response":{"subtype":"success","request_id":"usage","response":{}}}`), &out, "2.1.296", 4096, 8); err != nil {
		t.Fatal(err)
	}
}

func TestNativeSDKProcessUsesIsolationAndCloses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "native")
	script := `#!/bin/sh
case " $* " in *" --no-session-persistence "*) ;; *) exit 9 ;; esac
test "$REPORT_TEST" = isolated || exit 8
read -r request
printf '%s\n' '{"type":"control_response","response":{"subtype":"success","request_id":"initialize","response":{"claude_code_version":"2.1.296"}}}'
read -r request
case "$request" in *'"skip_behaviors":true'*) ;; *) exit 7 ;; esac
printf '%s\n' '{"type":"control_response","response":{"subtype":"success","request_id":"usage","response":{}}}'
read -r wait
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if data, err := ReadUsage(ctx, path, []string{"REPORT_TEST=isolated"}, dir, "2.1.296", 4096, 8); err != nil || string(data) != "{}" {
		t.Fatal(string(data), err)
	}
	if _, err := ReadUsage(ctx, path+"missing", nil, dir, "2.1.296", 4096, 8); err == nil {
		t.Fatal("missing process started")
	}
	old := newCommand
	t.Cleanup(func() { newCommand = old })
	for _, pipe := range []string{"stdout", "stdin"} {
		newCommand = func(ctx context.Context, path string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, path, args...)
			if pipe == "stdout" {
				cmd.Stdout = io.Discard
			} else {
				cmd.Stdin = strings.NewReader("")
			}
			return cmd
		}
		if _, err := ReadUsage(ctx, path, nil, dir, "2.1.296", 4096, 8); err == nil {
			t.Fatal(pipe)
		}
	}
}

func TestVersionGateRefusesUnknownPrereleaseAndOlderVersions(t *testing.T) {
	for _, tc := range []struct {
		version, minimum string
		want             bool
	}{
		{"2.1.296", "2.1.296", true}, {"v2.1.297", "2.1.296", true}, {"3.0.0", "2.1.296", true}, {"2.2.0", "2.1.296", true}, {"2.1.295", "2.1.296", false}, {"unknown", "2.1.296", false}, {"2.1.296-beta", "2.1.296", false}, {"2.1.296", "unknown", false}, {"2.-1.296", "2.1.296", false}, {"2.01.296", "2.1.296", false},
	} {
		if got := VersionAtLeast(tc.version, tc.minimum); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestInitializationVersionStopsUsageBeforeAnUnsafeScan(t *testing.T) {
	for _, result := range []string{`{}`, `[]`, `{"claude_code_version":"2.1.295"}`, `{"claude_code_version":"2.1.296-beta"}`} {
		body := `{"type":"control_response","response":{"subtype":"success","request_id":"initialize","response":` + result + `}}`
		var out strings.Builder
		if _, err := readUsage(strings.NewReader(body), &out, "2.1.296", 4096, 8); err == nil || strings.Contains(out.String(), "get_usage") {
			t.Fatal(result, out.String(), err)
		}
	}
}
