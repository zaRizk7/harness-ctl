// Package sdkreport reads native subscription reports through the experimental
// Claude Agent SDK control protocol. It sends no prompt or transcript-scan request.
package sdkreport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// newCommand supplies the process boundary for synthetic failure tests.
var newCommand = exec.CommandContext

// VersionAtLeast compares three-part release versions. Unknown versions and
// prereleases are unsupported. minimum must name a verified control contract.
func VersionAtLeast(version, minimum string) bool {
	parse := func(s string) ([3]int, bool) {
		var result [3]int
		parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
		if len(parts) != len(result) {
			return result, false
		}
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || strconv.Itoa(n) != part {
				return result, false
			}
			result[i] = n
		}
		return result, true
	}
	actual, ok := parse(version)
	required, valid := parse(minimum)
	if !ok || !valid {
		return false
	}
	for i := range actual {
		if actual[i] != required[i] {
			return actual[i] > required[i]
		}
	}
	return true
}

// ReadUsage starts executable with its selected env and dir. It disables user
// settings, hooks, tools, MCP and transcript persistence in the native request.
// minimum selects the required native version; initialization must confirm it.
// ctx, maxBytes and maxMessages bound the child and its output. Native managed
// policy still applies. Errors omit diagnostics, account data and credentials.
// It returns the native result object or a process/protocol error and closes the
// child before return.
func ReadUsage(ctx context.Context, executable string, env []string, dir, minimum string, maxBytes int64, maxMessages int) (json.RawMessage, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := newCommand(ctx, executable, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--setting-sources", "", "--settings", `{"disableAllHooks":true}`, "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--tools", "", "--no-session-persistence")
	cmd.Env = env
	cmd.Dir = dir
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("native SDK output pipe failed")
	}
	defer out.Close()
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("native SDK input pipe failed")
	}
	defer in.Close()
	if cmd.Start() != nil {
		return nil, fmt.Errorf("native SDK process failed to start")
	}
	defer func() { cancel(); cmd.Wait() }()
	return readUsage(out, in, minimum, maxBytes, maxMessages)
}

// readUsage accepts only ordered control replies. It refuses tool/auth requests
// and never writes a user message, so reporting cannot request an inference turn.
func readUsage(in io.Reader, out io.Writer, minimum string, maxBytes int64, maxMessages int) (json.RawMessage, error) {
	if maxBytes < 1 || maxMessages < 1 {
		return nil, fmt.Errorf("native SDK report limits must be positive")
	}
	if _, err := io.WriteString(out, "{\"type\":\"control_request\",\"request_id\":\"initialize\",\"request\":{\"subtype\":\"initialize\",\"hooks\":{}}}\n"); err != nil {
		return nil, fmt.Errorf("native SDK initialization failed")
	}
	decoder := json.NewDecoder(io.LimitReader(in, maxBytes))
	expected := "initialize"
	for count := 0; count < maxMessages; count++ {
		var message struct {
			Type     string `json:"type"`
			Response struct {
				Subtype string          `json:"subtype"`
				ID      string          `json:"request_id"`
				Data    json.RawMessage `json:"response"`
			} `json:"response"`
		}
		if decoder.Decode(&message) != nil {
			return nil, fmt.Errorf("native SDK report is unavailable or invalid")
		}
		if message.Type == "control_request" || message.Type == "assistant" || message.Type == "user" || message.Type == "result" {
			return nil, fmt.Errorf("unexpected native SDK request or turn")
		}
		if message.Type != "control_response" {
			continue
		}
		if message.Response.ID != expected || message.Response.Subtype != "success" {
			return nil, fmt.Errorf("native SDK method unavailable. Use a supported version and native subscription sign-in")
		}
		if expected == "initialize" {
			var result struct {
				Version string `json:"claude_code_version"`
			}
			if json.Unmarshal(message.Response.Data, &result) != nil || !VersionAtLeast(result.Version, minimum) {
				return nil, fmt.Errorf("native SDK initialization did not confirm a supported version")
			}
			if _, err := io.WriteString(out, "{\"type\":\"control_request\",\"request_id\":\"usage\",\"request\":{\"subtype\":\"get_usage\",\"skip_behaviors\":true}}\n"); err != nil {
				return nil, fmt.Errorf("native SDK report request failed")
			}
			expected = "usage"
			continue
		}
		if len(message.Response.Data) == 0 || string(message.Response.Data) == "null" {
			return nil, fmt.Errorf("native SDK report result is missing")
		}
		return message.Response.Data, nil
	}
	return nil, fmt.Errorf("native SDK report exceeds message limit")
}
