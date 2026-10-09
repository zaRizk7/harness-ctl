package manager

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zaRizk7/harness-ctl/internal/providers"
	"github.com/zaRizk7/harness-ctl/internal/vault"
)

// accountPath validates the owned vault location before either reading or writing.
func (e *engine) accountPath() (string, error) {
	p := filepath.Join(e.cfg.Root, "accounts.age")
	return p, validateOwnedPath(e.cfg.Root, p)
}

// loadAccounts authenticates and decrypts the vault. Missing vaults are empty.
// The plaintext exists only in memory and is never written to a temporary file.
func (e *engine) loadAccounts() ([]account, error) {
	path, err := e.accountPath()
	if err != nil {
		return nil, err
	}
	items := []account{}
	if err = vault.Read(path, e.cfg.MetadataBytes, e.identity, &items); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, a := range items {
		if err = e.validateAccount(a); err != nil {
			return nil, err
		}
		if seen[a.ID] {
			return nil, fmt.Errorf("duplicate account identity")
		}
		seen[a.ID] = true
	}
	return items, nil
}

// writeAccounts encrypts all accounts before publishing the atomic private vault.
// Callers hold the manager mutation lock, preventing lost updates and self-removal.
func (e *engine) writeAccounts(items []account) error {
	path, err := e.accountPath()
	if err != nil {
		return err
	}
	return vault.Write(path, e.cfg.MetadataBytes, e.identity, items)
}

// validateAccount rejects unsupported credential types and unsafe query identities.
func (e *engine) validateAccount(a account) error {
	if a.ReportInstallID != "" && a.ReportHarness == "" {
		return fmt.Errorf("native report installation requires a harness source")
	}
	if a.ReportHarness != "" {
		if a.Kind != "subscription" {
			return fmt.Errorf("native subscription source requires a subscription account")
		}
		if _, err := e.specFor(a.ReportHarness); err != nil {
			return err
		}
		provider, err := e.providerFor(a.Provider)
		if err != nil {
			return err
		}
		if a.ReportHarness != "codex" || provider.SubscriptionAdapter != "app_server" {
			return fmt.Errorf("selected provider/harness has no verified native subscription contract")
		}
	}
	if err := providers.ValidateExport(a.BillingExport); err != nil {
		return err
	}
	if !targetPattern.MatchString(a.ID) {
		return fmt.Errorf("invalid account ID")
	}
	if _, err := e.providerFor(a.Provider); err != nil {
		return err
	}
	if a.Kind != "api" && a.Kind != "subscription" {
		return fmt.Errorf("account kind must be api or subscription")
	}
	for _, v := range []string{a.Project, a.Organization} {
		if v != "" && !targetPattern.MatchString(v) {
			return fmt.Errorf("invalid project or organization ID")
		}
	}
	if strings.ContainsAny(a.Credential+a.MonitorCredential, "\r\n") {
		return fmt.Errorf("invalid credential")
	}
	return nil
}

// saveAccount adds or edits a and preserves its key when Credential is omitted.
func (e *engine) saveAccount(a account) error { return e.saveAccountChecked(a, "") }

// saveAccountChecked checks an editor's expected vault fingerprint under the
// mutation lock before editing. An empty expected value means a direct request.
func (e *engine) saveAccountChecked(a account, expected string) error {
	if err := e.validateAccount(a); err != nil {
		return err
	}
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err = e.checkAccountFingerprint(expected); err != nil {
		return err
	}
	items, err := e.loadAccounts()
	if err != nil {
		return err
	}
	for i, old := range items {
		if old.ID == a.ID {
			if a.Credential == "" {
				a.Credential = old.Credential
			}
			if a.MonitorCredential == "" {
				a.MonitorCredential = old.MonitorCredential
			}
			if old.Provider != a.Provider && ((old.Credential != "" && a.Credential == old.Credential) || (old.MonitorCredential != "" && a.MonitorCredential == old.MonitorCredential)) {
				return fmt.Errorf("changing providers requires replacing each stored credential")
			}
			items[i] = a
			return e.writeAccounts(items)
		}
	}
	return e.writeAccounts(append(items, a))
}

// removeAccount deletes id's local credential and metadata. It never revokes a
// remote provider key, cancels a subscription or modifies financial settings.
func (e *engine) removeAccount(id string) error {
	return e.removeAccountChecked(id, "")
}

// removeAccountChecked deletes id only when the approved vault is still current.
func (e *engine) removeAccountChecked(id, expected string) error {
	return e.changeAccount(id, nil, expected)
}

// setAccountEnabled toggles id without rewriting credentials from an earlier
// inventory read. A nonempty expected fingerprint binds it to a preview.
func (e *engine) setAccountEnabled(id string, enabled bool, expected string) error {
	return e.changeAccount(id, &enabled, expected)
}

// checkAccountFingerprint validates expected under the caller's mutation lock.
// An empty expected value selects the current vault without a prior preview.
func (e *engine) checkAccountFingerprint(expected string) error {
	if expected == "" {
		return nil
	}
	path, err := e.accountPath()
	if err != nil {
		return err
	}
	digest, err := fingerprint(path)
	if err != nil {
		return err
	}
	if digest != expected {
		return fmt.Errorf("account vault changed after selection. Refresh first")
	}
	return nil
}

// readAccountState returns decrypted records and their initial fingerprint.
// Concurrent replacement conservatively makes the resulting preview stale.
func (e *engine) readAccountState() ([]account, string, error) {
	path, err := e.accountPath()
	if err != nil {
		return nil, "", err
	}
	digest, err := fingerprint(path)
	if err != nil {
		return nil, "", err
	}
	items, err := e.loadAccounts()
	return items, digest, err
}

// changeAccount reads and changes id under one mutation lock. A nil enabled
// deletes it. Other values update only its enabled flag and preserve fresh keys.
func (e *engine) changeAccount(id string, enabled *bool, expected string) error {
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err = e.checkAccountFingerprint(expected); err != nil {
		return err
	}
	items, err := e.loadAccounts()
	if err != nil {
		return err
	}
	found := false
	kept := []account{}
	for _, a := range items {
		if a.ID == id {
			found = true
			if enabled != nil {
				a.Enabled = *enabled
				kept = append(kept, a)
			}
		} else {
			kept = append(kept, a)
		}
	}
	if !found {
		return fmt.Errorf("account does not exist")
	}
	return e.writeAccounts(kept)
}

// accountView is safe for inventory output and contains no credential values.
type accountView struct {
	ID, Provider, Kind, Label, Plan, Project, Organization string
	Enabled, HasCredential                                 bool
}

// accountViews converts decrypted records to a secret-free list.
func accountViews(items []account) []accountView {
	out := []accountView{}
	for _, a := range items {
		out = append(out, accountView{a.ID, a.Provider, a.Kind, a.Label, a.Plan, a.Project, a.Organization, a.Enabled, a.Credential != ""})
	}
	return out
}
