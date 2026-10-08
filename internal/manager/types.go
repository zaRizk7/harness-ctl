package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type category string

const (
	auth       category = "auth"
	settings   category = "settings"
	skills     category = "skills"
	plugins    category = "plugins"
	connectors category = "connectors"
	mcp        category = "mcp"
	hooks      category = "hooks"
	proxies    category = "proxies"
	history    category = "history"
	memory     category = "memory"
	cache      category = "cache"
	other      category = "other"
)

var categories = []category{auth, settings, skills, plugins, connectors, mcp, hooks, proxies, history, memory, cache, other}

type config struct {
	Root             string            `json:"root"`
	BinDir           string            `json:"bin_dir"`
	Home             string            `json:"home"`
	BackupDays       int               `json:"backup_days"`
	ProbeSeconds     int               `json:"probe_seconds"`
	OperationSeconds int               `json:"operation_seconds"`
	MaxSnapshotBytes int64             `json:"max_snapshot_bytes"`
	ReleaseChannel   string            `json:"release_channel"`
	StateRoots       map[string]string `json:"state_roots"`
	MetadataBytes    int64             `json:"metadata_bytes"`
	InstallerBytes   int64             `json:"installer_bytes"`
	PackageBytes     int64             `json:"package_bytes"`
}

func defaultConfig(home string) config {
	root := filepath.Join(home, "Library", "Application Support", "harness-ctl")
	return config{Root: root, BinDir: filepath.Join(root, "bin"), Home: home, BackupDays: 7, ProbeSeconds: 8, OperationSeconds: 1800, MaxSnapshotBytes: 4 << 30, MetadataBytes: 8 << 20, InstallerBytes: 4 << 20, PackageBytes: 512 << 20, ReleaseChannel: "stable", StateRoots: map[string]string{}}
}

func (c config) validate() error {
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

type command struct {
	Path        string
	Args        []string
	Env         map[string]string
	Dir         string
	Description string
}
type runner interface {
	Run(context.Context, command) (string, error)
}
type systemRunner struct{}

// Run executes a native adapter command with the supplied deadline and local Go policy.
func (systemRunner) Run(ctx context.Context, c command) (string, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	// Installers spawn children. Rollback must wait until their process group
	// has stopped, otherwise a child can overwrite the restored state.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
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

type harnessSpec struct {
	ID             string
	Name           string
	Command        string
	Package        string
	LegacyPackages []string
	BrewPackages   []string
	HomeEnv        string
	DefaultHome    string
	Kind           string
	Docs           string
	ConfigFiles    []string
	SharedClients  []string
	LaunchLabels   []string
}

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

type registry struct {
	Installs []installation     `json:"installations"`
	Profiles map[string]profile `json:"profiles"`
}
type profile struct {
	Disabled map[category]bool `json:"disabled"`
	Root     string            `json:"root"`
}

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
}

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
}

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

type engine struct {
	cfg       config
	run       runner
	reg       registry
	statePath string
	client    httpClient
	keys      keyStore
}

func newEngine(c config, r runner) (*engine, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	e := &engine{cfg: c, run: r, statePath: filepath.Join(c.Root, "registry.json"), keys: keychainStore{}, client: newHTTPClient(c)}
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

func keepAll() map[category]bool {
	m := map[category]bool{}
	for _, c := range categories {
		m[c] = true
	}
	return m
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func shortID(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
