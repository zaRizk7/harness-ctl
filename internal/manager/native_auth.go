package manager

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/zaRizk7/harness-ctl/internal/nativeauth"
)

// nativeAuthIO attaches approved native authentication to user-owned streams.
// No tokens, URLs, native output or callback state are recorded by the manager.
type nativeAuthIO struct {
	in  io.Reader
	out io.Writer
}

// planNativeAuth binds p to its native command and records shared-owner guards.
// Recovery covers captured local files only. Remote and OS credentials stay native.
func (e *engine) planNativeAuth(p *plan) error {
	args, err := nativeauth.Args(p.Spec.Auth, p.Request.Target, p.Request.AuthArgs)
	if err != nil {
		return err
	}
	c, err := e.launchCommand(p.Install, args, true)
	if err != nil {
		return err
	}
	p.Steps = []command{c}
	p.Warnings = append(p.Warnings, p.Spec.Auth.Notes[p.Request.Target], "Authentication stays with the native harness. Browser OAuth, remote sessions and OS credential changes cannot be reversed by local file recovery.")
	owners := append([]string{p.Spec.ID}, p.Spec.SharedClients...)
	for _, r := range p.Resources {
		owners = append(owners, r.Owners...)
	}
	if !ownersSelected(owners, p.Request) {
		p.Blockers = append(p.Blockers, "Select every affected native authentication owner with --owners or the owner screen.")
	}
	return nil
}

// nativeAuthDigest seals the approved operation, arguments, environment and
// ownership against in-memory edits before the transaction acquires its lock.
func nativeAuthDigest(p *plan) string {
	return valueDigest([]any{p.Request, p.Spec, p.Install, p.Steps, p.Resources, p.RootDigests, p.RegistryDigest, p.InstallDigest, p.Blockers, p.Credential})
}

// executeNativeAuth runs p through normal locking, stale checks, encrypted local
// recovery and cancellation. in/out belong to the interactive native terminal.
func (e *engine) executeNativeAuth(ctx context.Context, p *plan, approval string, in io.Reader, out io.Writer) error {
	copy := *e
	copy.authIO = &nativeAuthIO{in, out}
	return copy.execute(ctx, p, approval, nil)
}

// authCLI routes login/logout/status/management to installed native binaries.
// route resolves native prefixes forwarded by managed shims. Mutations require
// one concrete preview and approval. Native status streams without vault writes.
func (e *engine) authCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	if len(args) > 0 && args[0] == "profiles" {
		return e.credentialCLI(ctx, args[1:], in, out)
	}
	f := flag.NewFlagSet("auth", flag.ContinueOnError)
	f.SetOutput(out)
	id := f.String("install-id", "", "selected installation")
	owners := f.String("owners", "", "comma-separated affected owners")
	preview := f.Bool("preview", false, "show native command without executing")
	yes := f.Bool("yes", false, "approve the displayed native command")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() < 2 {
		return fmt.Errorf("auth [OPTIONS] login|logout|status|manage|usage|route HARNESS [NATIVE_OPTIONS...]")
	}
	op, harness := f.Arg(0), f.Arg(1)
	s, err := e.specFor(harness)
	if err != nil {
		return err
	}
	extra := f.Args()[2:]
	if op == "route" {
		op, extra = nativeauth.Match(s.Auth, extra)
		if op == "" {
			return fmt.Errorf("no verified native authentication route")
		}
	}
	if op == "status" {
		items, err := e.discover(ctx)
		if err != nil {
			return err
		}
		var selected installation
		for _, inst := range items {
			if inst.Harness == harness && (*id == "" || inst.ID == *id) {
				selected = inst
				if inst.Active || *id != "" {
					break
				}
			}
		}
		if selected.ID == "" {
			return fmt.Errorf("harness is not installed")
		}
		native, err := nativeauth.Args(s.Auth, op, extra)
		if err != nil {
			return err
		}
		c, err := e.launchCommand(selected, native, true)
		if err != nil {
			return err
		}
		if *preview {
			_, err := fmt.Fprintln(out, c.Path, strings.Join(c.Args, " "))
			return err
		}
		return interactiveCommand(ctx, c, in, out)
	}
	p, err := e.buildPlan(ctx, request{Harness: harness, InstallID: *id, Action: "auth", Target: op, AuthArgs: extra, Owners: strings.Split(*owners, ","), Preserve: keepAll()})
	if err != nil {
		return err
	}
	m := newModel(e)
	m.p = p
	if _, err := fmt.Fprintln(out, strings.Join(m.previewLines(), "\n")); err != nil {
		return err
	}
	if *preview {
		return nil
	}
	if err := confirmCLI(in, out, p.ID, *yes); err != nil {
		return err
	}
	return e.executeNativeAuth(ctx, p, p.ID, in, out)
}

// authShim writes native-prefix routing before native HOME/state exports. This
// lets the manager load its own user configuration and select the scoped process.
func (e *engine) authShim(body *strings.Builder, inst installation, s harnessSpec) error {
	prefixes := map[string]bool{}
	for _, args := range s.Auth.Commands {
		if len(args) > 0 {
			prefixes[args[0]] = true
		}
	}
	if len(prefixes) == 0 {
		return nil
	}
	binary, err := executablePath()
	if err != nil {
		return err
	}
	fmt.Fprintln(body, "case \"${1-}\" in")
	configArgs := "--root " + shellQuote(e.cfg.Root)
	if e.sourceConfig != "" {
		configArgs = "--config " + shellQuote(e.sourceConfig) + " " + configArgs
	}
	for _, prefix := range sortedKeys(prefixes) {
		fmt.Fprintf(body, "  %s) exec %s %s auth --install-id %s route %s \"$@\" ;;\n", shellQuote(prefix), shellQuote(binary), configArgs, shellQuote(inst.ID), shellQuote(s.ID))
	}
	fmt.Fprintln(body, "esac")
	return nil
}
