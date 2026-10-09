package manager

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// executablePath is the process executable boundary for self-removal ownership checks.
var executablePath = os.Executable

// selfPlan records the exact manager binary, optional harness batch and manager
// metadata selected for permanent removal. Harness payload/state stays separate.
type selfPlan struct {
	ID, Binary, BinaryDigest, RootDigest string
	RemoveState                          bool
	Batch                                *batchPlan
	Paths                                []string
	Links                                map[string]string
	Receipt                              string
	Digest                               string
}

// buildSelfPlan previews self-removal without writing. Harnesses are discovered
// only if their removal was selected. choices selects their preservation, affected owners and permanent erasure.
func (e *engine) buildSelfPlan(ctx context.Context, binary string, removeHarnesses, removeState bool, choices request) (*selfPlan, error) {
	if !filepath.IsAbs(binary) || !contains([]string{"harness-ctl", "harness-ctl-darwin-arm64", "harness-ctl-darwin-amd64"}, filepath.Base(binary)) {
		return nil, fmt.Errorf("select an absolute harness-ctl executable")
	}
	if err := validateOwnedPath(filepath.Dir(binary), binary); err != nil {
		return nil, err
	}
	info, err := fileIO.stat(binary)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return nil, fmt.Errorf("manager binary is not an executable file")
	}
	p := &selfPlan{ID: randomID(), Binary: binary, RemoveState: removeState, Links: map[string]string{}}
	var receipt struct{ Binary, Link string }
	receiptPath := filepath.Join(e.cfg.Root, "installation.json")
	if err := validateOwnedPath(e.cfg.Root, receiptPath); err != nil {
		return nil, err
	}
	if err := readJSON(receiptPath, &receipt); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if receipt.Binary == binary {
		p.Receipt = receiptPath
	}
	if receipt.Binary == binary && receipt.Link != "" {
		if filepath.Base(receipt.Link) != "harness-ctl" || !within(e.cfg.Home, receipt.Link) {
			return nil, fmt.Errorf("recorded launcher is outside user ownership")
		}
		if err := rejectLinkedAncestors(filepath.Dir(receipt.Link)); err != nil {
			return nil, err
		}
		target, err := fileIO.readlink(receipt.Link)
		if !os.IsNotExist(err) {
			if err != nil || target != binary {
				return nil, fmt.Errorf("recorded launcher target changed")
			}
			p.Links[receipt.Link], err = fingerprint(receipt.Link)
			if err != nil {
				return nil, err
			}
		}
	}
	p.BinaryDigest, err = fingerprint(binary)
	if err != nil {
		return nil, err
	}
	p.RootDigest, err = e.selfStateDigest()
	if err != nil {
		return nil, err
	}
	if removeHarnesses {
		items, err := e.discover(ctx)
		if err != nil {
			return nil, err
		}
		var reqs []request
		seen := map[string]bool{}
		for _, inst := range items {
			if inst.Method == "unsupported" {
				continue
			}
			if seen[inst.Harness] {
				return nil, fmt.Errorf("multiple installations of %s. Uninstall selected copies first", inst.Harness)
			}
			seen[inst.Harness] = true
			reqs = append(reqs, request{Harness: inst.Harness, InstallID: inst.ID, Action: "uninstall", Preserve: choices.Preserve, Owners: choices.Owners, Permanent: choices.Permanent})
		}
		if len(reqs) > 0 {
			p.Batch, err = e.buildBatch(ctx, reqs)
			if err != nil {
				return nil, err
			}
		}
	}
	if removeState {
		// These are exclusively manager metadata. Executables, harness auth and
		// launch profiles are not manager state, even when held beneath Root.
		for _, name := range []string{"registry.json", "accounts.age", "library.age", "catalog.json", "config.json", "installation.json", "snapshots", "operations", "downloads", "installer-homes", "restore-staging"} {
			path := filepath.Join(e.cfg.Root, name)
			if err := validateOwnedPath(e.cfg.Root, path); err != nil {
				return nil, err
			}
			p.Paths = append(p.Paths, path)
		}
	}
	p.Digest = selfPlanDigest(p)
	return p, nil
}

// selfPlanDigest binds removal choices and paths to the displayed approval.
func selfPlanDigest(p *selfPlan) string {
	return valueDigest([]any{p.ID, p.Binary, p.BinaryDigest, p.RootDigest, p.RemoveState, p.Batch, p.Paths, p.Links, p.Receipt})
}

// executeSelf removes p's executable last, after optional verified harness
// removal and manager metadata deletion. Failed harness operations preserve the
// manager so recovery remains available. Discarding manager state is permanent.
func (e *engine) executeSelf(ctx context.Context, p *selfPlan, approval string, progress func(string)) error {
	if p == nil || approval != p.ID || !safeID(p.ID) || p.Digest != selfPlanDigest(p) {
		return fmt.Errorf("self-uninstall requires approval of an unchanged preview")
	}
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	digest, err := e.selfStateDigest()
	if err != nil {
		return err
	}
	if digest != p.RootDigest {
		return fmt.Errorf("manager state changed after preview")
	}
	if err = e.ensureNoPending(); err != nil {
		return err
	}
	digest, err = fingerprint(p.Binary)
	if err != nil {
		return err
	}
	if digest != p.BinaryDigest {
		return fmt.Errorf("manager binary changed after preview")
	}
	for link, want := range p.Links {
		if err = rejectLinkedAncestors(filepath.Dir(link)); err != nil {
			return err
		}
		target, e := fileIO.readlink(link)
		if e != nil || target != p.Binary {
			return fmt.Errorf("recorded launcher changed after preview")
		}
		current, e := fingerprint(link)
		if e != nil {
			return e
		}
		if current != want {
			return fmt.Errorf("recorded launcher changed after preview")
		}
	}
	if p.Batch != nil {
		if err = e.executeBatchLocked(ctx, p.Batch, p.Batch.ID, progress); err != nil {
			return err
		}
	}
	if p.RemoveState {
		for _, path := range p.Paths {
			if err = validateOwnedPath(e.cfg.Root, path); err != nil {
				return err
			}
			if err = fileIO.removeAll(path); err != nil {
				return err
			}
		}
		if err = e.deleteStorageKey(); err != nil {
			return err
		}
	}
	if err = validateOwnedPath(filepath.Dir(p.Binary), p.Binary); err != nil {
		return err
	}
	if p.Receipt != "" {
		if err = fileIO.remove(p.Receipt); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for link := range p.Links {
		if err = fileIO.remove(link); err != nil {
			return err
		}
	}
	if err = fileIO.remove(p.Binary); err != nil {
		return err
	}
	return nil
}

// deleteStorageKey erases the manager identity after its encrypted files are
// permanently discarded. Test key stores implement the same deletion contract.
func (e *engine) deleteStorageKey() error {
	store, ok := e.keys.(interface{ Delete(string) error })
	if !ok {
		return fmt.Errorf("credential store does not support manager-key deletion")
	}
	err := store.Delete(installID("storage", e.cfg.Root))
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}

// Delete removes account from the manager keychain service and returns the native
// deletion error. The caller selects the current root's storage identity.
func (k keychainStore) Delete(account string) error { return k.remove(keychainService, account) }

// selfCLI previews independently selected harness and manager-state removal.
func (e *engine) selfCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("self-uninstall", flag.ContinueOnError)
	f.SetOutput(out)
	harnesses := f.Bool("harnesses", false, "also uninstall detected supported harnesses")
	state := f.Bool("state", false, "permanently discard manager metadata, accounts and recovery")
	preserve := f.String("preserve", "all", "harness categories to retain")
	yes := f.Bool("yes", false, "approve displayed self-removal")
	preview := f.Bool("preview", false, "preview only")
	owners := f.String("owners", "", "individually selected affected state owners")
	permanent := f.Bool("permanent", false, "erase harness discard recovery after successful removal")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected self-uninstall arguments")
	}
	binary, err := executablePath()
	if err != nil {
		return err
	}
	keep, err := parseCategories(*preserve)
	if err != nil {
		return err
	}
	p, err := e.buildSelfPlan(ctx, binary, *harnesses, *state, request{Preserve: keep, Owners: strings.Split(*owners, ","), Permanent: *permanent})
	if err != nil {
		return err
	}
	lines := []string{"Self-uninstall " + p.ID, "REMOVE " + p.Binary}
	if p.Receipt != "" {
		lines = append(lines, "REMOVE installation receipt "+p.Receipt)
	}
	for link := range p.Links {
		lines = append(lines, "REMOVE launcher "+link)
	}
	for _, path := range p.Paths {
		lines = append(lines, "PERMANENT REMOVE "+path)
	}
	lines = append(lines, "Retained harness payloads, user states and direct launch shims stay in place.")
	if _, err = fmt.Fprintln(out, strings.Join(lines, "\n")); err != nil {
		return err
	}
	if p.Batch != nil {
		if err = printBatch(out, p.Batch); err != nil {
			return err
		}
	}
	if *preview {
		return nil
	}
	if err = confirmCLI(in, out, p.ID, *yes); err != nil {
		return err
	}
	return e.executeSelf(ctx, p, p.ID, func(s string) { fmt.Fprintln(out, s) })
}

// selfStateDigest excludes only the synchronization inode from freshness checks.
// The digest is checked after lock acquisition, so lock creation cannot mask a
// concurrent account, registry or recovery edit.
func (e *engine) selfStateDigest() (string, error) {
	if err := rejectLinkedAncestors(e.cfg.Root); err != nil {
		return "", err
	}
	entries, err := fileIO.readDir(e.cfg.Root)
	if os.IsNotExist(err) {
		return valueDigest(map[string]string{}), nil
	}
	if err != nil {
		return "", err
	}
	values := map[string]string{}
	for _, entry := range entries {
		if entry.Name() == "operation.lock" {
			continue
		}
		digest, err := fingerprint(filepath.Join(e.cfg.Root, entry.Name()))
		if err != nil {
			return "", err
		}
		values[entry.Name()] = digest
	}
	return valueDigest(values), nil
}
