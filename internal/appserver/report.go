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
	return readLimits(out, in, maxBytes, maxMessages)
}

// readLimits completes the handshake before reading limits. It rejects server
// requests and malformed responses without creating an inference session.
func readLimits(in io.Reader, out io.Writer, maxBytes int64, maxMessages int) (json.RawMessage, error) {
	if maxBytes < 1 || maxMessages < 1 {
		return nil, fmt.Errorf("native report limits must be positive")
	}
	if _, err := io.WriteString(out, "{\"id\":1,\"method\":\"initialize\",\"params\":{\"clientInfo\":{\"name\":\"harness_ctl\",\"version\":\"1\"}}}\n"); err != nil {
		return nil, fmt.Errorf("native report initialization failed")
	}
	decoder := json.NewDecoder(io.LimitReader(in, maxBytes))
	initialized := false
	for count := 0; count < maxMessages; count++ {
		var message struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := decoder.Decode(&message); err != nil {
			return nil, fmt.Errorf("native report response is unavailable or invalid")
		}
		if message.ID == nil {
			continue
		}
		if message.Method != "" {
			return nil, fmt.Errorf("unexpected native report server request")
		}
		if *message.ID != 1 && *message.ID != 2 {
			return nil, fmt.Errorf("unexpected native report response identity")
		}
		if len(message.Error) > 0 && string(message.Error) != "null" {
			return nil, fmt.Errorf("native report method failed. Check installed version and native sign-in")
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
		return message.Result, nil
	}
	return nil, fmt.Errorf("native report exceeds message limit")
}
