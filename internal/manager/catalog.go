package manager

import (
	"fmt"
	"os"
	"path/filepath"
)

var catalog = []harnessSpec{
	{ID: "codex", Name: "Codex CLI", Command: "codex", Package: "@openai/codex", HomeEnv: "CODEX_HOME", DefaultHome: ".codex", Kind: "npm", Docs: "https://learn.chatgpt.com/docs/codex/cli", ConfigFiles: []string{"config.toml"}, SharedClients: []string{"Codex desktop / IDE"}},
	{ID: "claude", Name: "Claude Code", Command: "claude", Package: "@anthropic-ai/claude-code", HomeEnv: "CLAUDE_CONFIG_DIR", DefaultHome: ".claude", Kind: "npm", Docs: "https://code.claude.com/docs/en/setup", ConfigFiles: []string{"settings.json", "settings.local.json"}, SharedClients: []string{"Claude desktop / IDE"}},
	{ID: "gemini", Name: "Gemini CLI", Command: "gemini", Package: "@google/gemini-cli", DefaultHome: ".gemini", Kind: "npm", Docs: "https://geminicli.com/docs/get-started/installation/", ConfigFiles: []string{"settings.json"}},
	{ID: "opencode", Name: "OpenCode", Command: "opencode", Package: "opencode-ai", HomeEnv: "", DefaultHome: ".config/opencode", Kind: "npm", Docs: "https://opencode.ai/docs/", ConfigFiles: []string{"opencode.json", "opencode.jsonc"}},
	{ID: "pi", Name: "Pi", Command: "pi", Package: "@earendil-works/pi-coding-agent", LegacyPackages: []string{"@mariozechner/pi-coding-agent"}, HomeEnv: "PI_CODING_AGENT_DIR", DefaultHome: ".pi/agent", Kind: "npm", Docs: "https://github.com/earendil-works/pi", ConfigFiles: []string{"settings.json"}},
	{ID: "hermes", Name: "Hermes Agent", Command: "hermes", HomeEnv: "HERMES_HOME", DefaultHome: ".hermes", Kind: "hermes", Docs: "https://hermes-agent.nousresearch.com/docs/getting-started/installation", ConfigFiles: []string{"config.yaml"}, LaunchLabels: []string{"ai.hermes.gateway", "com.hermes.agent"}},
	{ID: "openclaw", Name: "OpenClaw", Command: "openclaw", Package: "openclaw", HomeEnv: "OPENCLAW_STATE_DIR", DefaultHome: ".openclaw", Kind: "npm", Docs: "https://docs.openclaw.ai/cli/reset", ConfigFiles: []string{"openclaw.json"}, LaunchLabels: []string{"ai.openclaw.gateway"}},
	{ID: "prime-agent", Name: "Prime Agent", Command: "prime-agent", HomeEnv: "PRIME_AGENT_CODING_AGENT_DIR", DefaultHome: ".prime/agent", Kind: "prime", Docs: "https://github.com/PrimeIntellect-ai/prime-agent", ConfigFiles: []string{"settings.json", "config.json", "config.toml"}},
}

var additionalCommands = []string{"amp", "aider", "droid", "cursor-agent", "goose", "qwen", "vibe", "kilo", "cline", "crush"}

func specFor(id string) (harnessSpec, error) {
	for _, s := range catalog {
		if s.ID == id {
			return s, nil
		}
	}
	return harnessSpec{}, fmt.Errorf("unsupported harness %q", id)
}

func (e *engine) stateRoot(s harnessSpec) string {
	if p := e.cfg.StateRoots[s.ID]; p != "" {
		return filepath.Clean(p)
	}
	if s.HomeEnv != "" {
		if p := os.Getenv(s.HomeEnv); filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
	}
	if s.ID == "opencode" {
		if root := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(root) {
			return filepath.Join(root, "opencode")
		}
	}
	return filepath.Join(e.cfg.Home, s.DefaultHome)
}

func (e *engine) rootsFor(s harnessSpec) []string {
	roots := []string{e.stateRoot(s)}
	if s.ID == "opencode" {
		if within(e.cfg.Root, roots[0]) {
			base := filepath.Dir(filepath.Dir(filepath.Dir(roots[0])))
			for _, p := range []string{"data", "state", "cache"} {
				roots = append(roots, filepath.Join(base, "xdg", p, "opencode"))
			}
		} else {
			for i, p := range []string{".local/share/opencode", ".local/state/opencode", ".cache/opencode"} {
				root := filepath.Join(e.cfg.Home, p)
				if override := os.Getenv([]string{"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"}[i]); filepath.IsAbs(override) {
					root = filepath.Join(override, "opencode")
				}
				roots = append(roots, root)
			}
		}
	}
	return roots
}

func nativeStateRoot(s harnessSpec, base string) string {
	if s.ID == "gemini" {
		return filepath.Join(base, "home", ".gemini")
	}
	if s.ID == "opencode" {
		return filepath.Join(base, "xdg", "config", "opencode")
	}
	return base
}

func (e *engine) managedStateRoot(s harnessSpec) string {
	return nativeStateRoot(s, filepath.Join(e.cfg.Root, "states", s.ID))
}

func (e *engine) resourceDestination(s harnessSpec, sourceRoot, targetRoot string, r resource) (string, bool) {
	source := *e
	source.cfg.StateRoots = map[string]string{s.ID: sourceRoot}
	target := *e
	target.cfg.StateRoots = map[string]string{s.ID: targetRoot}
	sourceRoots := source.rootsFor(s)
	targetRoots := target.rootsFor(s)
	for i, root := range sourceRoots {
		if canonicalSystemPath(root) == canonicalSystemPath(r.Root) && within(root, r.Path) {
			rel, err := filepath.Rel(root, r.Path)
			if err == nil {
				return filepath.Join(targetRoots[i], rel), true
			}
		}
	}
	return "", false
}
