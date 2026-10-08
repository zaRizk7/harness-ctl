package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var targetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

func (e *engine) buildPlan(ctx context.Context, req request) (*plan, error) {
	registryDigest, err := fingerprint(e.statePath)
	if err != nil {
		return nil, err
	}
	if err = e.refreshRegistry(); err != nil {
		return nil, err
	}
	if req.Preserve == nil {
		req.Preserve = keepAll()
	} else {
		req.Preserve = cloneCategories(req.Preserve)
	}
	req.Disabled = cloneCategories(req.Disabled)
	req.Owners = append([]string{}, req.Owners...)
	s, err := specFor(req.Harness)
	if err != nil {
		return nil, err
	}
	p := &plan{ID: randomID(), Created: time.Now().UTC(), Request: req, Spec: s}
	if req.Preserve == nil {
		p.Request.Preserve = keepAll()
	}
	switch req.Action {
	case "install", "uninstall", "reinstall", "reset", "update", "migrate", "profile":
	default:
		return nil, fmt.Errorf("unknown lifecycle action")
	}
	if req.Target != "" && !targetPattern.MatchString(req.Target) {
		return nil, fmt.Errorf("invalid target version")
	}
	if req.Action != "install" {
		installs, err := e.discover(ctx)
		if err != nil {
			return nil, err
		}
		for _, inst := range installs {
			if inst.Harness == s.ID && (req.InstallID == inst.ID || req.InstallID == "") {
				p.Install = inst
				if req.InstallID != "" || inst.Active {
					break
				}
			}
		}
		if p.Install.ID == "" {
			return nil, fmt.Errorf("selected installation no longer exists")
		}
		if p.Install.Method == "unknown" && req.Action != "reset" {
			p.Blockers = append(p.Blockers, "Installation ownership is unverified. Choose an independently verified managed installation.")
		}
	}
	if req.Action == "migrate" && p.Install.Managed {
		p.Blockers = append(p.Blockers, "This installation is already isolated and tool-managed.")
	}
	if req.Action == "profile" && !p.Install.Managed {
		p.Blockers = append(p.Blockers, "Launch profiles require a managed installation.")
	}
	if req.Model == "" {
		p.Request.Model = "isolated"
	}
	if p.Install.ID != "" && req.Action != "migrate" {
		if p.Install.Managed {
			p.Request.Model = "isolated"
		} else {
			p.Request.Model = "tracked"
		}
	}
	if p.Request.Model != "isolated" && p.Request.Model != "tracked" {
		return nil, fmt.Errorf("invalid installation model")
	}
	if req.Action == "update" || req.Action == "reinstall" {
		if p.Install.Managed && len(p.Install.ServicePaths) > 0 {
			p.Blockers = append(p.Blockers, "Managed prefix replacement requires verified service rebinding. Remove the selected installation's LaunchAgent registration before updating or reinstalling.")
		}
		if s.ID == "hermes" && p.Request.Model == "tracked" {
			p.Blockers = append(p.Blockers, "Tracked Hermes update/reinstall requires verified native launcher rebinding. Migrate to an isolated managed installation, or use the native updater.")
		}
	}
	p.StateRoot = e.stateRoot(s)
	if p.Install.Managed {
		p.StateRoot = p.Install.StateRoot
	}
	if req.Action == "install" && p.Request.Model == "isolated" {
		p.StateRoot = e.managedStateRoot(s)
		for _, inst := range e.reg.Installs {
			if inst.Harness == s.ID {
				p.Blockers = append(p.Blockers, "A managed installation already exists. Select update or reinstall.")
			}
		}
	}
	resourceEngine := *e
	resourceEngine.cfg.StateRoots = map[string]string{}
	for id, root := range e.cfg.StateRoots {
		resourceEngine.cfg.StateRoots[id] = root
	}
	resourceEngine.cfg.StateRoots[s.ID] = p.StateRoot
	p.Resources, err = resourceEngine.resources(s)
	if err != nil {
		return nil, err
	}
	p.RootDigests = map[string]string{}
	for _, root := range resourceEngine.rootsFor(s) {
		p.RootDigests[root], err = fingerprint(root)
		if err != nil {
			return nil, err
		}
	}
	if p.Install.Managed {
		profileRoot := filepath.Join(e.cfg.Root, "profiles", p.Install.ID)
		profileEngine := resourceEngine
		profileEngine.cfg.StateRoots = map[string]string{s.ID: nativeStateRoot(s, profileRoot)}
		profileResources, err := profileEngine.resources(s)
		if err != nil {
			return nil, err
		}
		p.Resources = append(p.Resources, profileResources...)
		p.RootDigests[profileRoot], err = fingerprint(profileRoot)
		if err != nil {
			return nil, err
		}
	}
	if req.Action == "reset" {
		allKept := true
		for _, cat := range categories {
			if !p.Request.Preserve[cat] {
				allKept = false
			}
		}
		if allKept {
			p.Warnings = append(p.Warnings, "Every category is preserved. This reset changes no user state.")
		}
	}
	for _, r := range p.Resources {
		if r.Linked {
			p.Warnings = append(p.Warnings, "Keep linked source: "+r.Path)
		}
		if !ownersSelected(r.Owners, p.Request) {
			p.Warnings = append(p.Warnings, "Keep shared source: "+r.Path+" ("+strings.Join(r.Owners, ", ")+")")
		}
		if shouldChange(r, p.Request) {
			if err := validateOwnedPath(r.Root, r.Path); err != nil {
				p.Blockers = append(p.Blockers, err.Error())
			}
		}
	}
	if !p.Request.Preserve[auth] && (s.ID == "codex" || s.ID == "claude") {
		if !ownersSelected(append([]string{s.ID}, s.SharedClients...), p.Request) || p.Request.Model == "isolated" {
			p.Warnings = append(p.Warnings, "Shared product authentication remains intact. Managed reset affects file state only.")
		} else if !p.Request.Permanent {
			p.Blockers = append(p.Blockers, "OS credential-store removal has no verified export contract. Preserve authentication, or select permanent discard with all affected clients selected.")
		} else if p.Install.Path != "" {
			args := []string{"logout"}
			if s.ID == "claude" {
				args = []string{"auth", "logout"}
			}
			p.Steps = append(p.Steps, command{Path: p.Install.Path, Args: args, Description: "Remove local credentials using native logout"})
			p.Warnings = append(p.Warnings, "Native logout cannot be undone for OS-held credentials.")
		}
	}
	if p.Request.Permanent {
		p.Warnings = append(p.Warnings, "Permanent discard removes manager-held snapshots containing discarded categories, including other recovery data in those archives.")
	}
	if len(s.SharedClients) > 0 && p.Request.Model == "tracked" {
		p.Warnings = append(p.Warnings, "Close affected desktop and IDE clients before applying. Preserved shared files are included in encrypted rollback recovery.")
	}
	if len(p.Blockers) == 0 && req.Action != "reset" && req.Action != "uninstall" && req.Action != "profile" {
		if err := e.installRecipe(ctx, p); err != nil {
			return nil, err
		}
	}
	if req.Action == "uninstall" && len(p.Blockers) == 0 {
		if err := e.uninstallRecipe(p, p.Install); err != nil {
			return nil, err
		}
	}
	if req.Action == "migrate" && !req.RemoveOld {
		p.Warnings = append(p.Warnings, "The old installation will be retained. A later uninstall completes migration.")
	}
	if req.Action == "migrate" && req.RemoveOld {
		p.Blockers = append(p.Blockers, "Migration retains the original installation until the new launch is checked. Uninstall it in a separate preview.")
	}
	if req.Action == "install" && p.Request.Model == "tracked" {
		p.Blockers = append(p.Blockers, "New installations use an isolated prefix. Track an existing installation through discovery.")
	}
	if p.Request.Permanent {
		p.Warnings = append(p.Warnings, "An encrypted rollback snapshot exists during execution and is erased after successful completion or rollback. An interrupted operation retains it until recovery.")
	}
	if s.ID == "gemini" && p.Request.Model == "isolated" {
		p.Warnings = append(p.Warnings, "Gemini isolation uses a shim-scoped HOME. Environment credentials and project/system sources remain native.")
	}
	for _, service := range p.Install.ServicePaths {
		if err = e.validateService(p.Install, service); err != nil {
			p.Blockers = append(p.Blockers, err.Error())
		}
	}
	if p.Install.Root != "" {
		p.InstallDigest, err = fingerprint(p.Install.Root)
		if err != nil {
			return nil, err
		}
	}
	p.RegistryDigest, err = fingerprint(e.statePath)
	if err != nil {
		return nil, err
	}
	if p.RegistryDigest != registryDigest {
		return nil, fmt.Errorf("registry changed while constructing preview. Try again")
	}
	return p, nil
}

func (e *engine) installRecipe(ctx context.Context, p *plan) error {
	target := p.Request.Target
	if p.Request.Action == "reinstall" && target == "" && p.Install.Method != "brew" {
		target = p.Install.Version
		if target == "unknown" {
			return fmt.Errorf("current version is unavailable. Enter an exact supported target version.")
		}
	}
	if p.Spec.Kind == "npm" {
		if p.Install.Method == "brew" && p.Request.Model == "tracked" {
			if target != "" {
				p.Blockers = append(p.Blockers, "Homebrew chooses the packaged release. Clear the target version.")
			}
			brew, err := lookPath("brew")
			if err != nil {
				return err
			}
			action := "upgrade"
			if p.Request.Action == "reinstall" {
				action = "reinstall"
			}
			p.Steps = append(p.Steps, command{Path: brew, Args: []string{action, p.Install.Package}, Description: "Apply Homebrew lifecycle action"})
			p.Destination = p.Install.Root
			p.Warnings = append(p.Warnings, "Homebrew binary rollback requires its package manager. State recovery remains available.")
			return nil
		}
		if p.Request.Model == "tracked" && strings.HasPrefix(p.Install.Method, "native-") {
			return e.nativeRecipe(ctx, p, target)
		}
		pkg := p.Spec.Package
		if p.Install.Method == "npm" && p.Install.Package != "" && p.Request.Action != "migrate" {
			pkg = p.Install.Package
		}
		if target == "" {
			var metadata struct {
				Tags map[string]string `json:"dist-tags"`
			}
			if err := e.getJSON(ctx, "https://registry.npmjs.org/"+url.PathEscape(pkg), &metadata); err != nil {
				return err
			}
			target = metadata.Tags["latest"]
		}
		if !targetPattern.MatchString(target) {
			return fmt.Errorf("upstream returned an invalid version")
		}
		var meta struct {
			Version string `json:"version"`
			Dist    struct {
				Integrity string `json:"integrity"`
				Tarball   string `json:"tarball"`
			} `json:"dist"`
		}
		if err := e.getJSON(ctx, "https://registry.npmjs.org/"+url.PathEscape(pkg)+"/"+url.PathEscape(target), &meta); err != nil {
			return err
		}
		if meta.Version != target || meta.Dist.Integrity == "" || meta.Dist.Tarball == "" {
			return fmt.Errorf("package metadata lacks version or integrity")
		}
		p.Integrity = meta.Dist.Integrity
		p.PackageURL = meta.Dist.Tarball
		p.Artifact = filepath.Join(e.cfg.Root, "downloads", p.ID+".tgz")
		p.Request.Target = target
		p.Destination = filepath.Join(e.cfg.Root, "installs", p.Spec.ID, target+"-"+shortID(p.ID))
		npm, err := lookPath("npm")
		if err != nil {
			npm = "npm"
			if err = e.requireBrewDependency(p, "node"); err != nil {
				return err
			}
		}
		args := []string{"install", "--global", "--no-audit", "--no-fund", "--prefix", p.Destination, p.Artifact}
		if p.Request.Model == "tracked" {
			prefix := ""
			if p.Install.Method == "npm" {
				prefix = strings.TrimSuffix(p.Install.Root, "/lib/node_modules/"+p.Install.Package)
			} else {
				out, err := e.run.Run(ctx, command{Path: npm, Args: []string{"prefix", "--global"}, Description: "Read npm prefix"})
				if err != nil {
					return err
				}
				prefix = strings.TrimSpace(out)
			}
			if !filepath.IsAbs(prefix) || prefix == "/" {
				return fmt.Errorf("npm prefix is unverified")
			}
			p.Destination = prefix
			args = []string{"install", "--global", "--no-audit", "--no-fund", "--prefix", prefix, p.Artifact}
			p.Warnings = append(p.Warnings, "Tracked npm installation changes the selected prefix. Runtime dependencies remain externally managed.")
		}
		p.Steps = append(p.Steps, command{Path: npm, Args: args, Description: "Install verified npm package " + pkg + "@" + target})
		p.Warnings = append(p.Warnings, "npm dependency resolution and lifecycle scripts execute upstream code with your user account. Shared runtimes remain externally managed.")
		return nil
	}
	return e.nativeRecipe(ctx, p, target)
}

func (e *engine) requireBrewDependency(p *plan, pkg string) error {
	brew, err := lookPath("brew")
	if err != nil {
		return fmt.Errorf("install dependency %s yourself, or install Homebrew before this operation", pkg)
	}
	p.DependencyCommands = append(p.DependencyCommands, command{Path: brew, Args: []string{"install", pkg}, Description: "Install missing shared dependency " + pkg})
	p.Warnings = append(p.Warnings, "Install shared prerequisite through Homebrew: "+pkg+". It will remain after harness removal.")
	return nil
}

func (e *engine) nativeRecipe(ctx context.Context, p *plan, target string) error {
	s := p.Spec
	p.Request.Target = target
	scriptURL := ""
	env := map[string]string{}
	args := []string{}
	if p.Request.Model == "isolated" {
		p.Destination = filepath.Join(e.cfg.Root, "installs", s.ID, "release-"+shortID(p.ID))
	} else {
		p.Destination = p.Install.Root
	}
	switch s.ID {
	case "codex":
		scriptURL = "https://chatgpt.com/codex/install.sh"
		env["CODEX_NON_INTERACTIVE"] = "1"
		env["CODEX_INSTALL_DIR"] = filepath.Dir(p.Install.Path)
		env["CODEX_HOME"] = e.stateRoot(s)
		env["CODEX_RELEASE"] = target
	case "claude":
		if p.Install.Path == "" {
			return fmt.Errorf("native Claude installation needs a verified existing path")
		}
		if target == "" {
			target = "latest"
			p.Request.Target = target
		}
		p.Steps = append(p.Steps, command{Path: p.Install.Path, Args: []string{"install", target, "--force"}, Description: "Reinstall or update native Claude Code"})
		return nil
	case "prime-agent":
		scriptURL = "https://app.primeintellect.ai/prime-agent/install.sh"
		if target == "" {
			data, err := e.get(ctx, "https://pub-728493de92a943e2a9b2d17b4719f318.r2.dev/stable", 1024)
			if err != nil {
				return err
			}
			target = strings.TrimPrefix(strings.TrimSpace(string(data)), "v")
		}
		if !targetPattern.MatchString(target) {
			return fmt.Errorf("invalid Prime Agent release")
		}
		p.Request.Target = target
		if p.Request.Model == "tracked" && p.Destination == "" {
			p.Destination = filepath.Join(e.cfg.Home, ".local/share/prime-agent")
		}
		env["PRIME_AGENT_VERSION"] = target
		env["PRIME_AGENT_RELEASE_CHANNEL"] = "stable"
		env["PRIME_AGENT_INSTALL_DIR"] = p.Destination
		env["PRIME_AGENT_INSTALL_LINK"] = "0"
		env["PRIME_AGENT_INSTALLER_NONINTERACTIVE"] = "1"
		env["PRIME_AGENT_INSTALL_METHOD"] = "binary"
	case "hermes":
		scriptURL = "https://hermes-agent.nousresearch.com/install.sh"
		if target == "" {
			var commit struct {
				SHA string `json:"sha"`
			}
			if err := e.getJSON(ctx, "https://api.github.com/repos/NousResearch/hermes-agent/commits/main", &commit); err != nil {
				return err
			}
			target = commit.SHA
		}
		if len(target) != 40 || !targetPattern.MatchString(target) {
			return fmt.Errorf("Hermes target must be an exact upstream commit SHA")
		}
		p.Request.Target = target
		if p.Request.Model == "tracked" && p.Destination == "" {
			p.Destination = filepath.Join(e.stateRoot(s), "hermes-agent")
		}
		if p.Install.Root != "" {
			out, err := e.run.Run(ctx, command{Path: "git", Args: []string{"-C", p.Install.Root, "status", "--porcelain"}, Description: "Check source checkout cleanliness"})
			if err != nil {
				return err
			}
			if strings.TrimSpace(out) != "" {
				p.Blockers = append(p.Blockers, "Hermes source has local changes. Commit or move them before reinstalling or updating.")
			}
		}
		stateRoot := p.StateRoot
		if p.Request.Action == "migrate" {
			stateRoot = e.managedStateRoot(s)
		}
		env["HERMES_HOME"] = stateRoot
		args = []string{"--dir", p.Destination, "--commit", target, "--non-interactive"}
		env["HERMES_RUNTIME_DIR"] = filepath.Join(p.Destination, "runtime")
		env["UV_PYTHON_INSTALL_DIR"] = filepath.Join(p.Destination, "python")
		p.Warnings = append(p.Warnings, "Hermes uses its native pinned Python/uv manager. Runtime acquisition and installation receipts belong to the previewed directories.")
	default:
		return fmt.Errorf("native installation recipe is unavailable for %s", s.Name)
	}
	if s.ID == "prime-agent" || s.ID == "hermes" {
		home := filepath.Join(e.cfg.Root, "installer-homes", p.ID)
		env["HOME"] = home
		env["XDG_CACHE_HOME"] = filepath.Join(home, ".cache")
		env["XDG_DATA_HOME"] = filepath.Join(home, ".local", "share")
		stateRoot := p.StateRoot
		if p.Request.Action == "migrate" {
			stateRoot = e.managedStateRoot(s)
		}
		if s.HomeEnv != "" {
			env[s.HomeEnv] = stateRoot
		}
		p.Warnings = append(p.Warnings, "Installer HOME and XDG writes are scoped to "+home+". Native runtime downloads may execute upstream code.")
	}
	data, err := e.get(ctx, scriptURL, e.cfg.InstallerBytes)
	if err != nil {
		return err
	}
	p.NativeScript = data
	sum := sha256.Sum256(data)
	p.Integrity = "sha256:" + hex.EncodeToString(sum[:])
	p.Artifact = filepath.Join(e.cfg.Root, "downloads", p.ID+".sh")
	if s.ID == "hermes" {
		for _, stage := range []string{"repository", "python-deps"} {
			stageArgs := append([]string{p.Artifact}, args...)
			stageArgs = append(stageArgs, "--stage", stage)
			p.Steps = append(p.Steps, command{Path: "bash", Args: stageArgs, Env: env, Description: "Hermes native " + stage + " stage"})
		}
	} else {
		shell := "sh"
		if s.ID == "hermes" {
			shell = "bash"
		}
		p.Steps = append(p.Steps, command{Path: shell, Args: append([]string{p.Artifact}, args...), Env: env, Description: "Run reviewed native installer"})
	}
	p.Warnings = append(p.Warnings, "Native installer payload is pinned to the previewed digest. Its own runtime acquisition remains native to the harness.")
	return nil
}

func (e *engine) uninstallRecipe(p *plan, inst installation) error {
	if inst.Managed {
		return nil
	}
	switch inst.Method {
	case "npm":
		npm, err := lookPath("npm")
		if err != nil {
			return err
		}
		prefix := strings.TrimSuffix(inst.Root, "/lib/node_modules/"+inst.Package)
		p.Steps = append(p.Steps, command{Path: npm, Args: []string{"uninstall", "--global", "--prefix", prefix, inst.Package}, Description: "Remove selected global npm package"})
	case "brew":
		brew, err := lookPath("brew")
		if err != nil {
			return err
		}
		p.Steps = append(p.Steps, command{Path: brew, Args: []string{"uninstall", inst.Package}, Description: "Remove selected Homebrew package"})
		p.Warnings = append(p.Warnings, "Homebrew binary rollback requires reinstalling its packaged version.")
	case "native-codex", "native-claude", "native-hermes", "native-prime":
		if inst.Root == "" || inst.Path == "" {
			return fmt.Errorf("native ownership is incomplete")
		}
		if err := validateOwnedPath(filepath.Dir(inst.Root), inst.Root); err != nil {
			return err
		}
	default:
		return fmt.Errorf("uninstall is unavailable for an unverified installation")
	}
	return nil
}

func (e *engine) validatePlan(p *plan) error {
	if len(p.Blockers) > 0 {
		return fmt.Errorf("operation is blocked: %s", strings.Join(p.Blockers, "; "))
	}
	digest, err := fingerprint(e.statePath)
	if err != nil {
		return err
	}
	if digest != p.RegistryDigest {
		return fmt.Errorf("registry changed after preview. Create a new preview.")
	}
	if p.Install.Root != "" {
		digest, err = fingerprint(p.Install.Root)
		if err != nil {
			return err
		}
		if digest != p.InstallDigest {
			return fmt.Errorf("installation changed after preview. Create a new preview.")
		}
	}
	for _, r := range p.Resources {
		digest, err = fingerprint(r.Path)
		if err != nil {
			return err
		}
		if digest != r.Digest {
			return fmt.Errorf("state changed after preview: %s", r.Path)
		}
	}
	for root, want := range p.RootDigests {
		digest, err = fingerprint(root)
		if err != nil {
			return err
		}
		if digest != want {
			return fmt.Errorf("state inventory changed after preview: %s", root)
		}
	}
	if len(p.NativeScript) > 0 {
		sum := sha256.Sum256(p.NativeScript)
		if "sha256:"+hex.EncodeToString(sum[:]) != p.Integrity {
			return fmt.Errorf("installer integrity changed")
		}
	}
	return nil
}
