// Package appserver reads subscription limits through the local JSONL protocol.
package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
)

// newCommand binds process creation for deterministic pipe/start failure tests.
var newCommand = exec.CommandContext

// ReadLimits starts executable with args/env and performs only initialization and
// account/rateLimits/read. ctx bounds the process and maxBytes/maxMessages bound
// responses. It returns the raw result object or a protocol/process error and
// stops the child before returning. Diagnostics and credentials stay out of errors.
func ReadLimits(ctx context.Context, executable string, args, env []string, maxBytes int64, maxMessages int) (json.RawMessage, error) {
	return readProcess(ctx, executable, args, env, maxBytes, maxMessages, false)
}

// ReadAccount reads quota windows and optional account token activity. An older
// server's unsupported usage method leaves valid quotas intact and adds a note.
// Arguments and process/response bounds match ReadLimits. It never starts a turn.
func ReadAccount(ctx context.Context, executable string, args, env []string, maxBytes int64, maxMessages int) (json.RawMessage, error) {
	return readProcess(ctx, executable, args, env, maxBytes, maxMessages, true)
}

// readProcess owns one native child and closes it before returning its report.
func readProcess(ctx context.Context, executable string, args, env []string, maxBytes int64, maxMessages int, activity bool) (json.RawMessage, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := newCommand(ctx, executable, args...)
	cmd.Env = env
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("native report output pipe failed")
	}
	defer out.Close()
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("native report input pipe failed")
	}
	defer in.Close()
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("native report process failed to start")
	}
	defer func() { cancel(); cmd.Wait() }()
	return readAccount(out, in, maxBytes, maxMessages, activity)
}

// readLimits completes the handshake before reading limits. It rejects server
// requests and malformed responses without creating an inference session.
func readLimits(in io.Reader, out io.Writer, maxBytes int64, maxMessages int) (json.RawMessage, error) {
	return readAccount(in, out, maxBytes, maxMessages, false)
}

// readAccount adds one optional usage read after quotas. Failure to read optional
// activity cannot erase verified quotas. Neither request creates a session turn.
func readAccount(in io.Reader, out io.Writer, maxBytes int64, maxMessages int, activity bool) (json.RawMessage, error) {
	if maxBytes < 1 || maxMessages < 1 {
		return nil, fmt.Errorf("native report limits must be positive")
	}
	if _, err := io.WriteString(out, "{\"id\":1,\"method\":\"initialize\",\"params\":{\"clientInfo\":{\"name\":\"harness_ctl\",\"version\":\"1\"}}}\n"); err != nil {
		return nil, fmt.Errorf("native report initialization failed")
	}
	decoder := json.NewDecoder(io.LimitReader(in, maxBytes))
	initialized := false
	var quota map[string]json.RawMessage
	unavailable := func() (json.RawMessage, error) {
		quota["usageNote"] = json.RawMessage(`"Native account token activity unavailable; use a supported version and native account sign-in"`)
		data, _ := json.Marshal(quota) // Raw values came from the JSON decoder.
		return data, nil
	}
	for count := 0; count < maxMessages; count++ {
		var message struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := decoder.Decode(&message); err != nil {
			if quota != nil {
				return unavailable()
			}
			return nil, fmt.Errorf("native report response is unavailable or invalid")
		}
		if message.ID == nil {
			continue
		}
		if message.Method != "" {
			return nil, fmt.Errorf("unexpected native report server request")
		}
		if *message.ID != 1 && *message.ID != 2 && !(activity && quota != nil && *message.ID == 3) {
			return nil, fmt.Errorf("unexpected native report response identity")
		}
		if len(message.Error) > 0 && string(message.Error) != "null" {
			if *message.ID == 3 && quota != nil {
				return unavailable()
			}
			return nil, fmt.Errorf("native report method failed. Check installed version and native sign-in")
		}
		if *message.ID == 3 && quota != nil {
			if len(message.Result) == 0 || string(message.Result) == "null" {
				return unavailable()
			}
			quota["usage"] = message.Result
			data, _ := json.Marshal(quota)
			return data, nil
		}
		if *message.ID == 1 && !initialized {
			if _, err := io.WriteString(out, "{\"method\":\"initialized\",\"params\":{}}\n{\"id\":2,\"method\":\"account/rateLimits/read\"}\n"); err != nil {
				return nil, fmt.Errorf("native report request failed")
			}
			initialized = true
			continue
		}
		if *message.ID != 2 || !initialized || len(message.Result) == 0 || string(message.Result) == "null" {
			return nil, fmt.Errorf("native report result is missing or out of order")
		}
		if !activity {
			return message.Result, nil
		}
		if quota != nil || json.Unmarshal(message.Result, &quota) != nil || quota == nil {
			return nil, fmt.Errorf("native quota report must be an object")
		}
		if _, err := io.WriteString(out, "{\"id\":3,\"method\":\"account/usage/read\"}\n"); err != nil {
			return unavailable()
		}
	}
	if quota != nil {
		return unavailable()
	}
	return nil, fmt.Errorf("native report exceeds message limit")
}
