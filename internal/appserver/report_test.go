package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAccountReportReadsQuotaAndTokenActivityWithoutInference(t *testing.T) {
	var out strings.Builder
	body := "{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"rateLimits\":{}}}\n{\"id\":3,\"result\":{\"summary\":{\"lifetimeTokens\":42}}}"
	data, err := readAccount(strings.NewReader(body), &out, 4096, 8, true)
	if err != nil || !json.Valid(data) || !strings.Contains(string(data), `"lifetimeTokens":42`) || !strings.Contains(out.String(), "account/usage/read") || strings.Contains(out.String(), "thread/start") {
		t.Fatal(string(data), out.String(), err)
	}
}

func TestOptionalAccountUsageFailureRetainsQuota(t *testing.T) {
	prefix := "{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"rateLimits\":{\"limitId\":\"account\"}}}\n"
	for _, suffix := range []string{"", "bad", `{"id":3,"error":{"message":"private diagnostic"}}`, `{"id":3}`, `{"id":3,"result":null}`} {
		data, err := readAccount(strings.NewReader(prefix+suffix), io.Discard, 4096, 8, true)
		if err != nil || !strings.Contains(string(data), `"limitId":"account"`) || !strings.Contains(string(data), "usageNote") || strings.Contains(string(data), "private diagnostic") {
			t.Fatal(suffix, string(data), err)
		}
	}
	for _, writer := range []io.Writer{io.Discard, &failWriter{after: 2}} {
		data, err := readAccount(strings.NewReader(prefix), writer, 4096, 2, true)
		if err != nil || !strings.Contains(string(data), "usageNote") {
			t.Fatal(string(data), err)
		}
	}
	for _, result := range []string{`[]`, `"string"`, `4`} {
		if _, err := readAccount(strings.NewReader("{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":"+result+"}"), io.Discard, 4096, 8, true); err == nil {
			t.Fatal(result)
		}
	}
	if _, err := readAccount(strings.NewReader(prefix+`{"id":2,"result":{}}`), io.Discard, 4096, 8, true); err == nil {
		t.Fatal("duplicate quota reply accepted")
	}
}

func TestReadAccountOwnsTheNativeProcess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "native")
	script := "#!/bin/sh\nread -r request\nprintf '%s\\n' '{\"id\":1,\"result\":{}}'\nread -r notice\nread -r request\nprintf '%s\\n' '{\"id\":2,\"result\":{}}'\nread -r request\nprintf '%s\\n' '{\"id\":3,\"result\":{\"summary\":{\"lifetimeTokens\":0}}}'\nread -r wait\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if data, err := ReadAccount(ctx, path, nil, nil, 4096, 8); err != nil || !strings.Contains(string(data), "lifetimeTokens") {
		t.Fatal(string(data), err)
	}
}

type failWriter struct{ after int }

func (w *failWriter) Write(p []byte) (int, error) {
	w.after--
	if w.after < 0 {
		return 0, errors.New("synthetic")
	}
	return len(p), nil
}

func TestHandshakeReadsOnlyNativeLimits(t *testing.T) {
	var out strings.Builder
	data, err := readLimits(strings.NewReader("{\"method\":\"notice\"}\n{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"rateLimits\":{}}}"), &out, 1024, 8)
	if err != nil || string(data) != `{"rateLimits":{}}` || !strings.Contains(out.String(), "account/rateLimits/read") || strings.Contains(out.String(), "thread/start") {
		t.Fatal(string(data), out.String(), err)
	}
	for _, tc := range []struct {
		body     string
		bytes    int64
		messages int
	}{
		{"", 0, 1}, {"", 1, 0}, {"invalid", 1024, 3}, {`{"id":1,"method":"request"}`, 1024, 3}, {`{"id":9}`, 1024, 3}, {`{"id":1,"error":{"secret":"hidden"}}`, 1024, 3}, {`{"id":2,"result":{}}`, 1024, 3}, {"{\"id\":1,\"result\":{}}\n{\"id\":1,\"result\":{}}", 1024, 3}, {"{\"id\":1,\"result\":{}}\n{\"id\":2}", 1024, 3}, {"{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":null}", 1024, 3}, {`{"method":"notice"}`, 1024, 1}, {`{"id":1,"result":{}}`, 8, 2},
	} {
		if _, err := readLimits(strings.NewReader(tc.body), io.Discard, tc.bytes, tc.messages); err == nil {
			t.Fatal(tc)
		}
	}
	for _, after := range []int{0, 1} {
		if _, err := readLimits(strings.NewReader(`{"id":1,"result":{}}`), &failWriter{after: after}, 1024, 3); err == nil {
			t.Fatal(after)
		}
	}
}

func TestNativeProcessIsBoundedAndClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "native")
	script := "#!/bin/sh\nread -r request\nprintf '%s\\n' '{\"id\":1,\"result\":{}}'\nread -r notification\nread -r request\nprintf '%s\\n' '{\"id\":2,\"result\":{\"rateLimits\":{}}}'\nread -r wait\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if result, err := ReadLimits(ctx, path, nil, nil, 1024, 4); err != nil || string(result) != `{"rateLimits":{}}` {
		t.Fatal(string(result), err)
	}
	if _, err := ReadLimits(ctx, filepath.Join(t.TempDir(), "missing"), nil, nil, 1024, 4); err == nil {
		t.Fatal("missing executable")
	}
	old := newCommand
	t.Cleanup(func() { newCommand = old })
	for _, stream := range []string{"stdout", "stdin"} {
		newCommand = func(ctx context.Context, path string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, path, args...)
			if stream == "stdout" {
				cmd.Stdout = io.Discard
			} else {
				cmd.Stdin = strings.NewReader("")
			}
			return cmd
		}
		if _, err := ReadLimits(ctx, path, nil, nil, 1024, 4); err == nil {
			t.Fatal(stream)
		}
	}
}
