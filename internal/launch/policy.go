// Package launch owns approval defaults and their POSIX launcher representation.
package launch

import (
	"fmt"
	"strings"
)

// Rule supplies one independent default option. Flags override this option.
// ConfigKeys match assignments supplied through ConfigFlags, never unrelated keys.
type Rule struct {
	Args       []string `json:"args"`
	Flags      []string `json:"flags,omitempty"`
	ConfigKeys []string `json:"config_keys,omitempty"`
}

// Policy controls independent defaults for both direct and shim launches.
// LegacyArgs/Flags retain older user catalogs until they adopt Rules.
type Policy struct {
	Rules                                []Rule
	ConfigFlags, LegacyArgs, LegacyFlags []string
}

// rules returns explicit rules or the legacy catalog's indivisible policy.
// Generic config flags are excluded from legacy whole-policy suppression.
func (p Policy) rules() []Rule {
	if len(p.Rules) > 0 {
		return p.Rules
	}
	var flags []string
	for _, f := range p.LegacyFlags {
		if f != "--config" && f != "-c" {
			flags = append(flags, f)
		}
	}
	return []Rule{{Args: p.LegacyArgs, Flags: flags}}
}

// DefaultArgs returns an independent copy of all configured default arguments.
func (p Policy) DefaultArgs() []string {
	result := []string{}
	for _, r := range p.rules() {
		result = append(result, r.Args...)
	}
	return result
}

// Args returns defaults followed by supplied arguments, removing only overridden
// rules. Native/help/version omit defaults. A -- terminator ends option scanning.
func (p Policy) Args(args []string, native bool) []string {
	flags, keys := p.options(args)
	if native || flags["--help"] || flags["-h"] || flags["--version"] || flags["-v"] {
		return append([]string{}, args...)
	}
	result := []string{}
	for _, r := range p.rules() {
		overridden := false
		for _, f := range r.Flags {
			overridden = overridden || flags[f]
		}
		for _, k := range r.ConfigKeys {
			overridden = overridden || keys[k]
		}
		if !overridden {
			result = append(result, r.Args...)
		}
	}
	return append(result, args...)
}

// options collects option names and explicit configuration keys from args.
// Separate, equals and attached short config values share the same parsing rule.
func (p Policy) options(args []string) (map[string]bool, map[string]bool) {
	flags, keys := map[string]bool{}, map[string]bool{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		name, _, _ := strings.Cut(arg, "=")
		flags[name] = true
		for _, r := range p.rules() {
			for _, f := range r.Flags {
				if len(f) == 2 && strings.HasPrefix(arg, f) {
					flags[f] = true
				}
			}
		}
		for _, f := range p.ConfigFlags {
			value := ""
			switch {
			case arg == f:
				if i+1 < len(args) {
					i++
					value = args[i]
				}
			case strings.HasPrefix(arg, f+"="):
				value = strings.TrimPrefix(arg, f+"=")
			case len(f) == 2 && strings.HasPrefix(arg, f):
				value = strings.TrimPrefix(arg, f)
			}
			if k, _, ok := strings.Cut(value, "="); ok {
				keys[strings.TrimSpace(k)] = true
			}
		}
	}
	return flags, keys
}

// Quote encodes value as a literal POSIX shell argument without evaluation.
func Quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

// Shell returns a POSIX script fragment that prepends the same defaults as Args.
// HARNESS_CTL_NATIVE=1 opts out. note is printed only for a default-policy launch.
func (p Policy) Shell(note string) string {
	var b strings.Builder
	b.WriteString("use_native=${HARNESS_CTL_NATIVE:-0}\n")
	rules := p.rules()
	for i := range rules {
		fmt.Fprintf(&b, "rule_%d=1\n", i)
	}
	b.WriteString("pending_config=0\nfor arg in \"$@\"; do\n  key=\n  value=\n  if [ \"$pending_config\" = 1 ]; then\n    value=$arg\n    pending_config=0\n  else\n    case \"$arg\" in\n      --) break ;;\n      --help|-h|--version|-v) use_native=1 ;;\n")
	for _, f := range p.ConfigFlags {
		fmt.Fprintf(&b, "      %s) pending_config=1 ;;\n      %s=*) value=${arg#*=} ;;\n", Quote(f), Quote(f))
		if len(f) == 2 {
			fmt.Fprintf(&b, "      %s*) value=${arg#%s} ;;\n", Quote(f), Quote(f))
		}
	}
	b.WriteString("    esac\n")
	for i, r := range rules {
		for _, f := range r.Flags {
			fmt.Fprintf(&b, "    case \"$arg\" in %s|%s=*) rule_%d=0 ;; esac\n", Quote(f), Quote(f), i)
			if len(f) == 2 {
				fmt.Fprintf(&b, "    case \"$arg\" in %s*) rule_%d=0 ;; esac\n", Quote(f), i)
			}
		}
	}
	b.WriteString("  fi\n")
	b.WriteString("  case \"$value\" in *=*) key=${value%%=*}; key=${key#\"${key%%[![:space:]]*}\"}; key=${key%\"${key##*[![:space:]]}\"} ;; esac\n")
	for i, r := range rules {
		for _, k := range r.ConfigKeys {
			fmt.Fprintf(&b, "  case \"$key\" in %s) rule_%d=0 ;; esac\n", Quote(k), i)
		}
	}
	b.WriteString("done\nif [ \"$use_native\" != 1 ]; then\n")
	for i := len(rules) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "  if [ \"$rule_%d\" = 1 ]; then set --", i)
		for _, arg := range rules[i].Args {
			fmt.Fprintf(&b, " %s", Quote(arg))
		}
		b.WriteString(" \"$@\"; fi\n")
	}
	if note != "" {
		fmt.Fprintf(&b, "  printf '%%s\\n' %s >&2\n", Quote(note))
	}
	b.WriteString("fi\n")
	return b.String()
}
