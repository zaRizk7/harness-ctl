package manager

import (
	"context"
	"errors"
	"fmt"
	"github.com/zaRizk7/harness-ctl/internal/providers"
	"github.com/zaRizk7/harness-ctl/internal/stateconfig"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// category is the shared state classification protocol used by transactions.
type category = stateconfig.Category

const (
	auth         = stateconfig.Auth
	settings     = stateconfig.Settings
	skills       = stateconfig.Skills
	plugins      = stateconfig.Plugins
	marketplaces = stateconfig.Marketplaces
	connectors   = stateconfig.Connectors
	mcp          = stateconfig.MCP
	hooks        = stateconfig.Hooks
	proxies      = stateconfig.Proxies
	history      = stateconfig.History
	memory       = stateconfig.Memory
	cache        = stateconfig.Cache
	other        = stateconfig.Other
)

var categories = stateconfig.Categories

// config holds user-configurable paths, limits and native adapter contracts.
// Defaults are provided by defaultConfig and validated before engine creation.
type config struct {
	Providers          []providerSpec    `json:"providers"`
	MonitorWindowDays  int               `json:"monitor_window_days"`
	MonitorMaxPages    int               `json:"monitor_max_pages"`
	CatalogFile        string            `json:"catalog_file,omitempty"`
	Harnesses          []harnessSpec     `json:"harnesses"`
	AdditionalCommands []string          `json:"additional_commands"`
	RefreshSeconds     int               `json:"refresh_seconds"`
	Root               string            `json:"root"`
	BinDir             string            `json:"bin_dir"`
	Home               string            `json:"home"`
	BackupDays         int               `json:"backup_days"`
	ProbeSeconds       int               `json:"probe_seconds"`
	OperationSeconds   int               `json:"operation_seconds"`
	MaxSnapshotBytes   int64             `json:"max_snapshot_bytes"`
	ReleaseChannel     string            `json:"release_channel"`
	StateRoots         map[string]string `json:"state_roots"`
	MetadataBytes      int64             `json:"metadata_bytes"`
	InstallerBytes     int64             `json:"installer_bytes"`
	PackageBytes       int64             `json:"package_bytes"`
}

// defaultConfig returns independent documented defaults for home. Call validate
// before using paths or configurable contracts.
func defaultConfig(home string) config {
	root := filepath.Join(home, "Library", "Application Support", "harness-ctl")
	return config{Providers: defaultProviders(), MonitorWindowDays: 30, MonitorMaxPages: 100, Harnesses: defaultCatalog(), AdditionalCommands: []string{"amp", "aider", "droid", "cursor-agent", "goose", "qwen", "vibe", "kilo", "cline", "crush"}, RefreshSeconds: 5, Root: root, BinDir: filepath.Join(root, "bin"), Home: home, BackupDays: 7, ProbeSeconds: 8, OperationSeconds: 1800, MaxSnapshotBytes: 4 << 30, MetadataBytes: 8 << 20, InstallerBytes: 4 << 20, PackageBytes: 512 << 20, ReleaseChannel: "stable", StateRoots: map[string]string{}}
}

// validate checks configured storage, catalog and account boundaries. Unsafe
// paths, unsupported values and nonpositive limits return errors.
func (c config) validate() error {
	if err := validateProviders(c.Providers); err != nil {
		return err
	}
	if c.MonitorWindowDays < 1 || c.MonitorMaxPages < 1 {
		return fmt.Errorf("monitor window and page limit must be positive")
	}
	if err := validateCatalog(c.Harnesses); err != nil {
		return err
	}
	if c.RefreshSeconds < 1 {
		return fmt.Errorf("refresh_seconds must be positive")
	}
	for _, command := range c.AdditionalCommands {
		if !targetPattern.MatchString(command) {
			return fmt.Errorf("invalid additional command")
		}
	}
	for _, p := range []string{c.Root, c.BinDir, c.Home} {
		if !filepath.IsAbs(p) || filepath.Clean(p) == "/" {
			return fmt.Errorf("configured directories must be absolute and cannot be /")
		}
	}
	if c.Root == c.Home || c.BinDir == c.Home || within(c.Root, c.Home) {
		return fmt.Errorf("manager storage cannot contain the user home")
	}
	if c.BackupDays < 1 || c.ProbeSeconds < 1 || c.OperationSeconds < 1 || c.MaxSnapshotBytes < 1 {
		return fmt.Errorf("retention, timeouts and snapshot limit must be positive")
	}
	if c.ReleaseChannel != "stable" {
		return fmt.Errorf("v1 supports stable releases only")
	}
	if c.MetadataBytes < 1 || c.InstallerBytes < 1 || c.PackageBytes < 1 {
		return fmt.Errorf("download limits must be positive")
	}
	if err := validateOwnedPath(c.Root, c.BinDir); err != nil {
		return err
	}
	if err := rejectLinkedAncestors(c.Root); err != nil {
		return err
	}
	for id, p := range c.StateRoots {
		if !filepath.IsAbs(p) || filepath.Clean(p) == c.Home || filepath.Clean(p) == "/" {
			return fmt.Errorf("unsafe state root for %s", id)
		}
	}
	return nil
}

// command describes a native process, including arguments, working directory
// and environment overrides. Descriptions label operations without exposing secrets.
type command struct {
	Path        string
	Args        []string
	Env         map[string]string
	Dir         string
	Description string
}

// runner is the native-command boundary used by adapters and synthetic tests.
type runner interface {
	Run(context.Context, command) (string, error)
}

// systemRunner executes local processes and kills installer process groups on cancellation.
type systemRunner struct{}

// Run executes c.Path with c.Args, c.Dir and environment overrides under ctx's
// deadline and local Go policy. It returns combined stdout/stderr, including on
// failure, and wraps execution errors with c.Description. Cancellation stops the
// process group before returning so installers cannot write after rollback.
func (systemRunner) Run(ctx context.Context, c command) (string, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	// Installers spawn children. Rollback must wait until their process group
	// has stopped, otherwise a child can overwrite the restored state.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return stopProcessGroup(cmd.Process.Pid)
	}
	env := map[string]string{}
	for _, pair := range os.Environ() {
		k, v, ok := strings.Cut(pair, "=")
		if ok {
			env[k] = v
		}
	}
	// Every Go invocation, including a child reached through a recipe, uses
	// the user-installed compiler. Native harness self-updaters are disabled
	// during manager probes and managed launches where supported.
	env["GOTOOLCHAIN"] = "local"
	env["DISABLE_AUTOUPDATER"] = "1"
	for k, v := range c.Env {
		env[k] = v
	}
	env["GOTOOLCHAIN"] = "local"
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	data, err := cmd.CombinedOutput()
	if err != nil {
		return string(data), fmt.Errorf("%s failed: %w", c.Description, err)
	}
	return string(data), nil
}

// stopProcessGroup stops an installer's children before rollback. A process
// group that has already exited uses the native command cancellation sentinel.
func stopProcessGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

// installation records a discovered executable and its verified payload/state
// ownership. Managed identifies isolated prefixes owned by this manager.
type installation struct {
	ID           string   `json:"id"`
	Harness      string   `json:"harness"`
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	Root         string   `json:"root"`
	Version      string   `json:"version"`
	Managed      bool     `json:"managed"`
	Active       bool     `json:"active"`
	Package      string   `json:"package,omitempty"`
	ServicePaths []string `json:"service_paths,omitempty"`
	Note         string   `json:"note,omitempty"`
	StateRoot    string   `json:"state_root,omitempty"`
}

// registry records managed installs and per-installation launch profiles.
type registry struct {
	Installs []installation     `json:"installations"`
	Profiles map[string]profile `json:"profiles"`
}

// profile selects a private state copy and its excluded categories for a shim.
type profile struct {
	Disabled map[category]bool `json:"disabled"`
	Root     string            `json:"root"`
}

// resource binds a concrete state path to owners, categories and preview
// fingerprints. Fields classify independently editable structured settings.
type resource struct {
	Path         string              `json:"path"`
	Root         string              `json:"root"`
	Category     category            `json:"category"`
	Owners       []string            `json:"owners"`
	Format       string              `json:"format,omitempty"`
	Fields       map[string]category `json:"fields,omitempty"`
	FieldDigests map[string]string   `json:"field_fingerprints,omitempty"`
	Digest       string              `json:"fingerprint"`
	Linked       bool                `json:"linked,omitempty"`
	Note         string              `json:"note,omitempty"`
}

// request contains a lifecycle action and the selected preservation, owner
// and source-profile controls. It is input to read-only plan construction.
type request struct {
	Harness   string
	InstallID string
	Action    string
	Target    string
	Model     string
	Preserve  map[category]bool
	Permanent bool
	Owners    []string
	RemoveOld bool
	Disabled  map[category]bool
	Component *componentRequest
}

// plan is a previewed transaction with native commands, resource fingerprints
// and blockers. Execution revalidates it under the mutation lock.
type plan struct {
	ID                 string
	Created            time.Time
	Request            request
	Spec               harnessSpec
	Install            installation
	Resources          []resource
	Steps              []command
	Warnings           []string
	Blockers           []string
	Destination        string
	Artifact           string
	PackageURL         string
	Integrity          string
	NativeScript       []byte
	RegistryDigest     string
	InstallDigest      string
	DependencyCommands []command
	StateRoot          string
	RootDigests        map[string]string
	Component          *componentMutation
}

// operationRecord journals transaction progress and rollback coordinates.
// Unsettled records prevent a new mutation until recovery completes.
type operationRecord struct {
	ID          string    `json:"id"`
	Harness     string    `json:"harness"`
	Action      string    `json:"action"`
	Status      string    `json:"status"`
	Started     time.Time `json:"started"`
	Snapshot    string    `json:"snapshot,omitempty"`
	Error       string    `json:"error,omitempty"`
	Previous    *registry `json:"previous_registry,omitempty"`
	Services    []string  `json:"services,omitempty"`
	Destination string    `json:"destination,omitempty"`
}

// engine owns configuration, registry, native boundaries and transactional state.
// Scoped copies share reporting cooldowns and retain the same storage ownership.
type engine struct {
	cfg           config
	run           runner
	reg           registry
	statePath     string
	client        httpClient
	accountClient httpClient
	keys          keyStore
	reports       *providers.Reporter
}

// newEngine returns a read-only engine configured by c with command runner r.
// Invalid configuration or registry ownership returns an error.
func newEngine(c config, r runner) (*engine, error) {
	if c.CatalogFile == "" {
		path := filepath.Join(c.Root, "catalog.json")
		if _, err := fileIO.lstat(path); err == nil {
			c.CatalogFile = path
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if c.CatalogFile != "" {
		if err := rejectLinkedAncestors(c.CatalogFile); err != nil {
			return nil, err
		}
		if err := readJSON(c.CatalogFile, &c.Harnesses); err != nil {
			return nil, err
		}
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	c.Harnesses = normalizeCatalog(c.Harnesses)
	e := &engine{cfg: c, run: r, statePath: filepath.Join(c.Root, "registry.json"), keys: newKeychainStore(), client: newHTTPClient(c), accountClient: newAccountClient(c), reports: providers.New()}
	if err := validateOwnedPath(c.Root, e.statePath); err != nil {
		return nil, err
	}
	e.reg = registry{Profiles: map[string]profile{}}
	if err := readJSON(e.statePath, &e.reg); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if e.reg.Profiles == nil {
		e.reg.Profiles = map[string]profile{}
	}
	if err := e.validateRegistry(e.reg); err != nil {
		return nil, err
	}
	return e, nil
}

// keepAll returns a new preservation map containing every state category.
func keepAll() map[category]bool {
	m := map[category]bool{}
	for _, c := range categories {
		m[c] = true
	}
	return m
}

// contains reports whether xs includes x using exact string equality.
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// shortID returns a compact prefix of s for display. Approval always uses the
// complete identity.
func shortID(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
