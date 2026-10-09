package manager

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zaRizk7/harness-ctl/internal/credentials"
	"github.com/zaRizk7/harness-ctl/internal/vault"
)

// credentialMutation binds an approved local profile action to its encrypted
// vault and declared native files. It never includes OS-held credentials.
type credentialMutation struct {
	Action    string
	Path      string
	Profile   credentials.Profile
	Locations []credentials.Location
}

// credentialPath validates private manager ownership before accessing the vault.
func (e *engine) credentialPath() (string, error) {
	path := filepath.Join(e.cfg.Root, "credentials.age")
	return path, validateOwnedPath(e.cfg.Root, path)
}

// loadCredentialProfiles authenticates stored profile schemas and known harnesses.
// Obsolete contracts remain removable. Current file compatibility is checked on
// application. Missing storage returns an empty list and unsafe identities fail.
func (e *engine) loadCredentialProfiles() ([]credentials.Profile, error) {
	path, err := e.credentialPath()
	if err != nil {
		return nil, err
	}
	profiles := []credentials.Profile{}
	if err := vault.Read(path, e.cfg.MetadataBytes, e.identity, &profiles); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, p := range profiles {
		_, err := e.specFor(p.Harness)
		if err != nil {
			return nil, err
		}
		if err := credentials.ValidateStored(p); err != nil {
			return nil, err
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("duplicate native credential profile")
		}
		seen[p.ID] = true
	}
	return profiles, nil
}

// credentialState resolves the same native roots used by the selected launch.
// State-only sources support capture/restore after a harness has been removed.
func (e *engine) credentialState(p *plan) *engine {
	state := *e
	state.cfg.StateRoots = map[string]string{p.Spec.ID: p.StateRoot}
	return &state
}

// credentialFile validates loc against p's native roots before any file IO.
func (e *engine) credentialFile(p *plan, loc credentials.Location) (string, error) {
	roots := e.credentialState(p).rootsFor(p.Spec)
	if loc.Root < 0 || loc.Root >= len(roots) {
		return "", fmt.Errorf("native credential root is unavailable")
	}
	path := filepath.Join(roots[loc.Root], loc.Path)
	return path, validateOwnedPath(roots[loc.Root], path)
}

// planCredentials previews capture/apply/remove without exposing file contents.
// The vault joins local recovery so a failed operation can restore its prior copy.
func (e *engine) planCredentials(p *plan) error {
	if !targetPattern.MatchString(p.Request.CredentialID) {
		return fmt.Errorf("credential profile requires a valid ID")
	}
	if !contains([]string{"capture", "apply", "remove"}, p.Request.Target) {
		return fmt.Errorf("unknown credential profile action")
	}
	path, err := e.credentialPath()
	if err != nil {
		return err
	}
	profiles, err := e.loadCredentialProfiles()
	if err != nil {
		return err
	}
	profile := credentials.Profile{ID: p.Request.CredentialID, Harness: p.Spec.ID}
	found := false
	for _, item := range profiles {
		if item.ID == profile.ID {
			profile = item
			found = true
		}
	}
	if found && profile.Harness != p.Spec.ID {
		return fmt.Errorf("credential profile belongs to a different harness")
	}
	if p.Request.Target != "capture" && !found {
		return fmt.Errorf("credential profile does not exist")
	}
	if p.Request.Target == "capture" && len(p.Spec.Auth.CredentialFiles) == 0 {
		return fmt.Errorf("no verified file-backed credentials. Use native credential storage")
	}
	if p.Request.Target == "apply" {
		if err := credentials.Validate(profile, p.Spec.ID, p.Spec.Auth.CredentialFiles); err != nil {
			return fmt.Errorf("saved profile is incompatible with the current native credential contract. Authenticate natively or capture a new compatible profile")
		}
	}
	p.Credential = &credentialMutation{Action: p.Request.Target, Path: path, Profile: profile, Locations: p.Spec.Auth.CredentialFiles}
	digest, err := fingerprint(path)
	if err != nil {
		return err
	}
	vaultOwners := []string{p.Spec.ID}
	for _, stored := range profiles {
		if !contains(vaultOwners, stored.Harness) {
			vaultOwners = append(vaultOwners, stored.Harness)
		}
	}
	p.Resources = nil
	p.Resources = append(p.Resources, resource{Path: path, Root: e.cfg.Root, Category: auth, Owners: vaultOwners, Digest: digest, Note: "Encrypted native credential profiles"})
	if p.Request.Target != "remove" {
		owners := append([]string{p.Spec.ID}, p.Spec.SharedClients...)
		for _, r := range p.Resources {
			owners = append(owners, r.Owners...)
		}
		if !ownersSelected(owners, p.Request) {
			p.Blockers = append(p.Blockers, "Select every affected credential owner before capture or restore.")
		}
		for _, loc := range p.Spec.Auth.CredentialFiles {
			file, err := e.credentialFile(p, loc)
			if err != nil {
				return err
			}
			digest, err := fingerprint(file)
			if err != nil {
				return err
			}
			p.Resources = append(p.Resources, resource{Path: file, Root: e.credentialState(p).rootsFor(p.Spec)[loc.Root], Category: auth, Owners: owners, Digest: digest})
		}
	}
	p.Warnings = append(p.Warnings, "Profile: "+profile.ID, "Only declared native credential files are captured. OS secrets and remote sessions remain native. Restored tokens may be expired or revoked. Native login/refresh determines validity.")
	return nil
}

// applyCredentials applies p's validated action under the transaction lock.
// Payloads are encrypted before manager publication and written privately only
// to explicitly approved native destinations, with local rollback on failure.
func (e *engine) applyCredentials(p *plan) error {
	profiles, err := e.loadCredentialProfiles()
	if err != nil {
		return err
	}
	profile := p.Credential.Profile
	switch p.Credential.Action {
	case "apply":
		for _, file := range profile.Files {
			path, err := e.credentialFile(p, file.Location)
			if err != nil {
				return err
			}
			if err := atomicWrite(path, file.Data, 0600); err != nil {
				return err
			}
		}
		return nil
	case "capture":
		profile.Files = nil
		profile.Captured = time.Now().UTC()
		for _, loc := range p.Credential.Locations {
			path, err := e.credentialFile(p, loc)
			if err != nil {
				return err
			}
			info, err := fileIO.lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() > e.cfg.MetadataBytes {
				return fmt.Errorf("credential file must be regular and within the metadata limit")
			}
			file, err := fileIO.open(path)
			if err != nil {
				return err
			}
			data, err := io.ReadAll(io.LimitReader(file, e.cfg.MetadataBytes+1))
			closeErr := fileIO.close(file)
			if closeErr != nil {
				return closeErr
			}
			if int64(len(data)) > e.cfg.MetadataBytes {
				return fmt.Errorf("credential file grew beyond the metadata limit")
			}
			if err != nil {
				return err
			}
			profile.Files = append(profile.Files, credentials.File{Location: loc, Data: data})
		}
		if err := credentials.Validate(profile, p.Spec.ID, p.Spec.Auth.CredentialFiles); err != nil {
			return fmt.Errorf("no complete declared credentials to capture. Authenticate natively first. OS-only credentials stay native")
		}
		if err := e.validatePlan(p); err != nil {
			return err
		}
	}
	kept := []credentials.Profile{}
	for _, old := range profiles {
		if (p.Credential.Action == "purge" && old.Harness != p.Spec.ID) || (p.Credential.Action != "purge" && old.ID != profile.ID) {
			kept = append(kept, old)
		}
	}
	if p.Credential.Action == "capture" {
		kept = append(kept, profile)
	}
	return vault.Write(p.Credential.Path, e.cfg.MetadataBytes, e.identity, kept)
}

// credentialCLI lists secret-free profiles or approves native capture/apply/remove.
// Options precede the harness. Plain credential values never enter CLI arguments.
func (e *engine) credentialCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("auth profiles list|capture|apply|remove [--profile ID] [--owners OWNER,...] [--preview|--yes] HARNESS")
	}
	if args[0] == "list" {
		profiles, err := e.loadCredentialProfiles()
		if err != nil {
			return err
		}
		return outputJSON(out, credentials.Views(profiles))
	}
	f := flag.NewFlagSet("credential profile", flag.ContinueOnError)
	f.SetOutput(out)
	id := f.String("profile", "", "credential profile ID")
	install := f.String("install-id", "", "selected native installation")
	owners := f.String("owners", "", "affected credential owners")
	preview := f.Bool("preview", false, "show paths without applying")
	yes := f.Bool("yes", false, "approve the displayed local credential action")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return fmt.Errorf("select one compatible harness")
	}
	p, err := e.buildPlan(ctx, request{Harness: f.Arg(0), InstallID: *install, Action: "credentials", Target: args[0], CredentialID: *id, Owners: strings.Split(*owners, ","), Preserve: keepAll()})
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
	return e.execute(ctx, p, p.ID, nil)
}

// planCredentialPurge includes captured native copies in permanent auth discard.
// Its vault fingerprint and encrypted rollback remain part of the same transaction.
func (e *engine) planCredentialPurge(p *plan) error {
	if !p.Request.Permanent || p.Request.Preserve[auth] {
		return nil
	}
	profiles, err := e.loadCredentialProfiles()
	if err != nil {
		return err
	}
	found := false
	for _, profile := range profiles {
		if profile.Harness == p.Spec.ID {
			found = true
		}
	}
	if !found {
		return nil
	}
	path, err := e.credentialPath()
	if err != nil {
		return err
	}
	digest, err := fingerprint(path)
	if err != nil {
		return err
	}
	p.RootDigests[path] = digest
	p.Credential = &credentialMutation{Action: "purge", Path: path}
	p.Warnings = append(p.Warnings, "Permanent auth discard also erases this harness's manager-held credential profiles. Other harness profiles remain.")
	return nil
}
