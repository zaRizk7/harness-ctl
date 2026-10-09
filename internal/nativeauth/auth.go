// Package nativeauth owns configurable native authentication command routing.
// Credentials and OAuth callbacks remain with the selected harness process.
package nativeauth

import (
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/credentials"
	"slices"
	"strings"
)

// Spec declares native command prefixes and operator guidance by operation.
// Empty arguments open the native interface. Missing operations are unavailable.
type Spec struct {
	RequiredArgument map[string]string      `json:"required_argument,omitempty"`
	CredentialFiles  []credentials.Location `json:"credential_files,omitempty"`
	Commands         map[string][]string    `json:"commands,omitempty"`
	Notes            map[string]string      `json:"notes,omitempty"`
}

// Validate rejects unknown operations and control characters in configured args.
// Commands run as argument vectors, never through a shell or an inference loop.
func Validate(s Spec) error {
	if err := credentials.ValidateLocations(s.CredentialFiles); err != nil {
		return err
	}
	for op, args := range s.Commands {
		if !slices.Contains([]string{"login", "logout", "status", "manage", "usage"}, op) {
			return fmt.Errorf("unknown native auth operation %q", op)
		}
		for _, arg := range args {
			if strings.ContainsAny(arg, "\x00\r\n") {
				return fmt.Errorf("invalid native auth argument")
			}
		}
	}
	return nil
}

// Args returns independent native arguments for op plus explicitly supplied
// options. It rejects absent contracts and inline secrets before previewing them.
func Args(s Spec, op string, extra []string) ([]string, error) {
	if label := s.RequiredArgument[op]; label != "" && len(extra) == 0 {
		return nil, fmt.Errorf("native %s requires %s", op, label)
	}
	args, ok := s.Commands[op]
	if !ok {
		return nil, fmt.Errorf("no verified native %s contract. Use native management or configure the catalog", op)
	}
	for _, arg := range extra {
		name, _, _ := strings.Cut(strings.ToLower(arg), "=")
		if slices.Contains([]string{"--api-key", "--token", "--access-token", "--refresh-token", "--key"}, name) {
			return nil, fmt.Errorf("supply secrets through native prompts or stdin, never inline options")
		}
		if strings.ContainsAny(arg, "\x00\r\n") || strings.Contains(strings.ToLower(arg), "token=") || strings.Contains(strings.ToLower(arg), "key=") {
			return nil, fmt.Errorf("supply secrets through native prompts or stdin, never inline options")
		}
	}
	return append(append([]string{}, args...), extra...), nil
}

// Match returns the most specific nonempty command prefix matching args.
// For example login status takes precedence over login. Interactive entries
// cannot intercept normal sessions because their prefix is empty.
func Match(s Spec, args []string) (op string, extra []string) {
	longest := 0
	names := make([]string, 0, len(s.Commands))
	for name := range s.Commands {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		prefix := s.Commands[name]
		if len(prefix) > longest && len(args) >= len(prefix) && slices.Equal(args[:len(prefix)], prefix) {
			op, extra, longest = name, args[len(prefix):], len(prefix)
		}
	}
	return
}
