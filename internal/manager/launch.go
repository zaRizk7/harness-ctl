package manager

import (
	"context"
	"flag"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/launch"
	"github.com/zaRizk7/harness-ctl/internal/providers"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// launchPolicy returns the catalog's independent security defaults and overrides.
func launchPolicy(s harnessSpec) launch.Policy {
	return launch.Policy{Rules: s.LaunchRules, ConfigFlags: s.ConfigFlags, LegacyArgs: s.LaunchArgs, LegacyFlags: s.ApprovalFlags}
}

// defaultLaunchArgs retains independent defaults when args overrides one option.
// native and help/version omit defaults. Returned arguments do not alias args.
func defaultLaunchArgs(s harnessSpec, args []string, native bool) []string {
	return launchPolicy(s).Args(args, native)
}

// launchCommand resolves inst's state/profile environment and forwards args.
// It returns a command without starting a process or creating state directories.
func (e *engine) launchCommand(inst installation, args []string, native bool) (command, error) {
	s, err := e.specFor(inst.Harness)
	if err != nil {
		return command{}, err
	}
	if !filepath.IsAbs(inst.Path) || within(e.cfg.BinDir, inst.Path) {
		return command{}, fmt.Errorf("native executable is unavailable or recursive")
	}
	root := inst.StateRoot
	if root == "" {
		root = e.stateRoot(s)
	}
	prof, hasProfile := e.reg.Profiles[inst.ID]
	if hasProfile {
		root = nativeStateRoot(s, prof.Root)
	}
	env := e.launchEnvironment(inst, root)
	if hasProfile {
		env["HOME"] = filepath.Join(prof.Root, "home")
		if s.ID != "opencode" {
			for _, name := range []string{"CONFIG", "DATA", "STATE", "CACHE"} {
				env["XDG_"+name+"_HOME"] = filepath.Join(prof.Root, "xdg", strings.ToLower(name))
			}
		}
		if prof.Disabled[proxies] {
			for _, name := range proxyEnvironment {
				env[name] = ""
			}
		}
	}
	return command{Path: inst.Path, Args: defaultLaunchArgs(s, args, native), Env: env, Description: "Launch " + s.Name}, nil
}

var proxyEnvironment = []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"}

// launchCLI selects the active installation or explicit ID and starts its native
// interactive process with the supplied streams. Harness flags follow its ID.
func (e *engine) launchCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("launch", flag.ContinueOnError)
	f.SetOutput(out)
	accountID := f.String("account", "", "enabled API account to supply to this launch")
	native := f.Bool("native", false, "use native approval defaults")
	id := f.String("install-id", "", "installation identifier")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() < 1 {
		return fmt.Errorf("launch requires a harness")
	}
	harness := f.Arg(0)
	if _, err := e.specFor(harness); err != nil {
		return err
	}
	items, err := e.discover(ctx)
	if err != nil {
		return err
	}
	var selected installation
	for _, i := range items {
		if i.Harness == harness && (*id == "" || i.ID == *id) {
			selected = i
			if *id != "" || i.Active {
				break
			}
		}
	}
	if selected.ID == "" {
		return fmt.Errorf("harness is not installed")
	}
	forward := f.Args()[1:]
	if len(forward) > 0 && forward[0] == "--" {
		forward = forward[1:]
	}
	c, err := e.launchCommand(selected, forward, *native)
	if err != nil {
		return err
	}
	if *accountID != "" {
		env, err := e.accountLaunchEnvironment(*accountID)
		if err != nil {
			return err
		}
		for k, v := range env {
			if _, owned := c.Env[k]; owned {
				return fmt.Errorf("provider environment cannot override launch state")
			}
			c.Env[k] = v
		}
	}
	spec, _ := e.specFor(harness)
	if !*native && spec.LaunchNote != "" {
		fmt.Fprintln(os.Stderr, spec.LaunchNote)
	}
	return interactiveCommand(ctx, c, in, out)
}

// interactiveCommand attaches native stdin/stdout without buffering a running
// session. Environment values override inherited entries without duplicate keys.
func interactiveCommand(ctx context.Context, c command, in io.Reader, out io.Writer) error {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = out
	env := map[string]string{}
	for _, pair := range os.Environ() {
		k, v, ok := strings.Cut(pair, "=")
		if ok {
			env[k] = v
		}
	}
	for k, v := range c.Env {
		env[k] = v
	}
	for _, k := range sortedKeys(env) {
		cmd.Env = append(cmd.Env, k+"="+env[k])
	}
	return cmd.Run()
}

// accountLaunchEnvironment supplies only an enabled account's inference key.
// Reporting/admin keys and subscription records can never become harness keys.
func (e *engine) accountLaunchEnvironment(id string) (map[string]string, error) {
	items, err := e.loadAccounts()
	if err != nil {
		return nil, err
	}
	for _, a := range items {
		if a.ID == id {
			if !a.Enabled || a.Kind != "api" || a.Credential == "" {
				return nil, fmt.Errorf("select an enabled API account with an inference key")
			}
			// loadAccounts validated provider membership before returning records.
			p, _ := e.providerFor(a.Provider)
			return providers.LaunchEnvironment(p, a), nil
		}
	}
	return nil, fmt.Errorf("account does not exist")
}
