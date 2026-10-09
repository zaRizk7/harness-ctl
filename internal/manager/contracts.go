package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/appserver"
	harnesscatalog "github.com/zaRizk7/harness-ctl/internal/catalog"
	"github.com/zaRizk7/harness-ctl/internal/providers"
	"os"
	"strings"
	"time"
)

// harnessSpec is the catalog contract used by native lifecycle adapters.
type harnessSpec = harnesscatalog.Spec

// account is the provider-independent encrypted authentication record.
type account = providers.Account

// providerSpec is a configured native reporting and launch contract.
type providerSpec = providers.Spec

// accountMetric is the secret-free provider report shown by both interfaces.
type accountMetric = providers.Metric

// defaultCatalog returns fresh shipped adapter defaults.
func defaultCatalog() []harnessSpec { return harnesscatalog.Defaults() }

// validateCatalog checks catalog contract safety before native discovery.
func validateCatalog(s []harnessSpec) error { return harnesscatalog.Validate(s) }

// normalizeCatalog returns independent rules for unchanged shipped legacy policies.
func normalizeCatalog(s []harnessSpec) []harnessSpec { return harnesscatalog.NormalizeLegacy(s) }

// defaultProviders returns fresh shipped reporting contracts.
func defaultProviders() []providerSpec { return providers.Defaults() }

// validateProviders checks destinations and environment names before using keys.
func validateProviders(s []providerSpec) error { return providers.Validate(s) }

// providerFor resolves id against configured provider metadata.
func (e *engine) providerFor(id string) (providerSpec, error) {
	return providers.Find(e.cfg.Providers, id)
}

// reportConfig maps manager settings to the reporting module without mutations.
func (e *engine) reportConfig() providers.Config {
	return providers.Config{Specs: e.cfg.Providers, WindowDays: e.cfg.MonitorWindowDays, MaxPages: e.cfg.MonitorMaxPages, ResponseBytes: e.cfg.MetadataBytes, TimeoutSeconds: e.cfg.ProbeSeconds, RefreshSeconds: e.cfg.RefreshSeconds, NativeReport: e.nativeSubscriptionReport}
}

// newAccountClient configures the reporting boundary with refused redirects.
func newAccountClient(c config) httpClient {
	return providers.NewClient(providers.Config{TimeoutSeconds: c.ProbeSeconds})
}

// monitorAccount reads a's capabilities and native metrics at now.
func (e *engine) monitorAccount(ctx context.Context, a account, now time.Time) accountMetric {
	return e.reports.Monitor(ctx, a, now, e.reportConfig(), e.accountClient)
}

// nativeSubscriptionReport selects an installed catalog entry's native state and
// reads app-server limits. It never copies tokens or creates an inference turn.
func (e *engine) nativeSubscriptionReport(ctx context.Context, a account) (json.RawMessage, error) {
	if err := e.validateAccount(a); err != nil {
		return nil, err
	}
	// Load an independent read-only registry snapshot. Reporting must not race
	// transaction-owned in-memory registry/profile updates.
	state, err := newEngine(e.cfg, e.run)
	if err != nil {
		return nil, err
	}
	items, err := state.discover(ctx)
	if err != nil {
		return nil, err
	}
	var selected installation
	for _, inst := range items {
		if inst.Harness == a.ReportHarness && (a.ReportInstallID == "" || inst.ID == a.ReportInstallID) {
			selected = inst
			if inst.Active || a.ReportInstallID != "" {
				break
			}
		}
	}
	if selected.ID == "" {
		return nil, fmt.Errorf("subscription report harness is not installed")
	}
	c, err := state.launchCommand(selected, nil, true)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, pair := range os.Environ() {
		if k, v, ok := strings.Cut(pair, "="); ok {
			env[k] = v
		}
	}
	for k, v := range c.Env {
		env[k] = v
	}
	pairs := []string{}
	for _, key := range sortedKeys(env) {
		pairs = append(pairs, key+"="+env[key])
	}
	return appserver.ReadLimits(ctx, c.Path, []string{"app-server", "--listen", "stdio://"}, pairs, e.cfg.MetadataBytes, e.cfg.MonitorMaxPages)
}
