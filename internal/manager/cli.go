package manager

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

var version = "dev"

// hostOS is the native platform boundary used by startup checks.
var hostOS = runtime.GOOS

const usage = `harness-ctl [--config FILE] [--root DIR] COMMAND [OPTIONS]

No command opens the TUI.
  tui, list, catalog, config, version
  setup [--binary FILE --sha256 HASH --prefix DIR --link-dir DIR]
    [--shell-file FILE] [--headless]
  install|uninstall|reinstall|reset|update|upgrade|migrate [OPTIONS] HARNESS...
    --preserve all|none|auth|CATEGORY,...  --target VERSION  --owners OWNER,...
    --preview  --yes  --permanent  --install-id ID
  library list|set|enable|disable|remove|apply
  components list [--scope base|profile|source:NAME] HARNESS CATEGORY
  components apply [--yes|--preview] HARNESS REQUEST.json
  launch [--native] [--install-id ID] HARNESS [--] OPTIONS...
  HARNESS OPTIONS...                   launch shorthand
  path                                print shell PATH setup
  auth [--install-id ID] [--owners OWNER,...] [--preview|--yes]
    login|logout|status|manage|usage|route HARNESS [NATIVE_OPTIONS...]
  accounts list|set|enable|disable|remove|monitor|open
  self-uninstall [--harnesses] [--state] [--preserve CATEGORIES]
    [--owners OWNER,...] [--permanent] [--preview|--yes]

Mutations print a preview. --yes approves that concrete preview without a prompt.
Batch transactions apply in order and stop on failure. Go is user-managed.
`

// Main runs args using the process streams and returns an exit status. Global
// flags must precede the command. It never installs or acquires a Go toolchain.
func Main(args []string) int {
	return mainWithIO(args, os.Stdin, os.Stdout, os.Stderr)
}

// mainWithIO keeps parsing and command output testable without opening a TUI.
func mainWithIO(args []string, in io.Reader, out, errout io.Writer) int {
	flags := flag.NewFlagSet("harness-ctl", flag.ContinueOnError)
	flags.SetOutput(errout)
	configFile := flags.String("config", "", "JSON configuration path")
	root := flags.String("root", "", "override manager storage directory")
	flags.Usage = func() { fmt.Fprint(errout, usage); flags.PrintDefaults() }
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	rest := flags.Args()
	if len(rest) == 0 {
		rest = []string{"tui"}
	}
	if rest[0] == "version" {
		fmt.Fprintln(out, "harness-ctl", version)
		return 0
	}
	if hostOS != "darwin" {
		fmt.Fprintln(errout, "harness-ctl supports macOS only")
		return 1
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	cfg := defaultConfig(home)
	if *configFile != "" {
		if err = readJSON(*configFile, &cfg); err != nil {
			fmt.Fprintln(errout, err)
			return 1
		}
	}
	if *root != "" {
		cfg.Root = filepath.Clean(*root)
		cfg.BinDir = filepath.Join(cfg.Root, "bin")
	}
	if *configFile == "" {
		path := filepath.Join(cfg.Root, "config.json")
		if err = readJSON(path, &cfg); err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(errout, err)
			return 1
		}
		if *root != "" && cfg.Root != filepath.Clean(*root) {
			fmt.Fprintln(errout, "stored configuration root differs from selected root")
			return 1
		}
	}
	e, err := newEngine(cfg, systemRunner{})
	if err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	e.sourceConfig = *configFile
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if rest[0] == "tui" {
		err = runTUI(e)
	} else {
		err = e.cli(ctx, rest, in, out)
	}
	if err != nil {
		fmt.Fprintln(errout, err)
		return 1
	}
	return 0
}

// cli dispatches read-only commands, approved transactions and native launches.
// in supplies explicit approval and out receives previews, results and metadata.
func (e *engine) cli(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%s", usage)
	}
	switch args[0] {
	case "setup":
		return e.setupCLI(ctx, args[1:], in, out)
	case "config":
		return outputJSON(out, e.cfg)
	case "catalog":
		return outputJSON(out, e.cfg.Harnesses)
	case "list":
		items, err := e.discover(ctx)
		if err != nil {
			return err
		}
		return outputJSON(out, items)
	case "path":
		fmt.Fprintf(out, "export PATH=%s:\"$PATH\"\n", shellQuote(e.cfg.BinDir))
		return nil
	case "auth":
		return e.authCLI(ctx, args[1:], in, out)
	case "accounts":
		return e.accountsCLI(ctx, args[1:], in, out)
	case "self-uninstall":
		return e.selfCLI(ctx, args[1:], in, out)
	case "library":
		return e.libraryCLI(ctx, args[1:], in, out)
	case "components":
		return e.componentsCLI(ctx, args[1:], in, out)
	case "launch":
		return e.launchCLI(ctx, args[1:], in, out)
	case "install", "uninstall", "reinstall", "reset", "update", "upgrade", "migrate":
		return e.lifecycleCLI(ctx, args, in, out)
	default:
		if _, err := e.specFor(args[0]); err == nil {
			return e.launchCLI(ctx, args, in, out)
		}
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

// outputJSON writes a human-readable JSON result, returning serialization or IO errors.
func outputJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

// confirmCLI requires the displayed identifier unless yes was explicitly supplied.
// End-of-input and a mismatched identifier cancel without executing any mutation.
func confirmCLI(in io.Reader, out io.Writer, id string, yes bool) error {
	if yes {
		return nil
	}
	if _, err := fmt.Fprintf(out, "Type %s to apply, or anything else to cancel: ", id); err != nil {
		return err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if strings.TrimSpace(line) != id {
		return fmt.Errorf("operation cancelled")
	}
	return nil
}

// parseCategories parses a preservation preset or explicit comma-separated names.
func parseCategories(value string) (map[category]bool, error) {
	switch value {
	case "all":
		return keepAll(), nil
	case "none":
		return map[category]bool{}, nil
	}
	result := map[category]bool{}
	for _, name := range strings.Split(value, ",") {
		cat := category(name)
		if !knownCategory(cat) {
			return nil, fmt.Errorf("unknown category %q", name)
		}
		result[cat] = true
	}
	return result, nil
}

// lifecycleCLI builds every requested harness plan before requesting one batch approval.
func (e *engine) lifecycleCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	action := args[0]
	if action == "upgrade" {
		action = "update"
	}
	f := flag.NewFlagSet(action, flag.ContinueOnError)
	f.SetOutput(out)
	preset := f.String("preserve", "all", "categories to preserve")
	target := f.String("target", "", "target version")
	owners := f.String("owners", "", "comma-separated affected owners")
	install := f.String("install-id", "", "specific installation (one harness only)")
	permanent := f.Bool("permanent", false, "erase recovery after successful discard")
	preview := f.Bool("preview", false, "print preview without applying")
	yes := f.Bool("yes", false, "approve displayed batch")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() == 0 {
		return fmt.Errorf("select at least one harness")
	}
	if *install != "" && f.NArg() != 1 {
		return fmt.Errorf("install-id requires one harness")
	}
	keep, err := parseCategories(*preset)
	if err != nil {
		return err
	}
	var requests []request
	for _, id := range f.Args() {
		req := request{Harness: id, Action: action, InstallID: *install, Target: *target, Preserve: keep, Permanent: *permanent}
		if *owners != "" {
			req.Owners = strings.Split(*owners, ",")
		}
		requests = append(requests, req)
	}
	b, err := e.buildBatch(ctx, requests)
	if err != nil {
		return err
	}
	if err = printBatch(out, b); err != nil {
		return err
	}
	if *preview {
		return nil
	}
	if err = confirmCLI(in, out, b.ID, *yes); err != nil {
		return err
	}
	return e.executeBatch(ctx, b, b.ID, func(s string) { fmt.Fprintln(out, s) })
}

// printBatch renders paths and commands without emitting component/auth values.
func printBatch(out io.Writer, b *batchPlan) error {
	if _, err := fmt.Fprintf(out, "Batch %s: %d operation(s), in order. Stops on failure.\n", b.ID, len(b.Plans)); err != nil {
		return err
	}
	for _, p := range b.Plans {
		m := tuiModel{p: p, req: p.Request, height: 1 << 30}
		if _, err := fmt.Fprintln(out, strings.Join(m.previewLines(), "\n")); err != nil {
			return err
		}
	}
	return nil
}

// componentsCLI lists named items or applies a JSON component request through
// the same scoped transaction and secret-free preview used by the TUI.
func (e *engine) componentsCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("components list HARNESS CATEGORY | apply [--yes|--preview] HARNESS REQUEST.json")
	}
	f := flag.NewFlagSet("components", flag.ContinueOnError)
	scope := f.String("scope", "base", "selected component inventory source")
	f.SetOutput(out)
	yes := f.Bool("yes", false, "approve displayed preview")
	preview := f.Bool("preview", false, "preview only")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 2 {
		return fmt.Errorf("components requires harness and category or request file")
	}
	id := f.Arg(0)
	if _, err := e.specFor(id); err != nil {
		return err
	}
	if args[0] == "list" {
		cat := category(f.Arg(1))
		if !knownCategory(cat) {
			return fmt.Errorf("unknown category")
		}
		inst := installation{Harness: id}
		items, err := e.discover(ctx)
		if err != nil {
			return err
		}
		for _, i := range items {
			if i.Harness == id {
				inst = i
				if i.Active {
					break
				}
			}
		}
		entries, err := e.components(inst, *scope, cat)
		if err != nil {
			return err
		}
		return outputJSON(out, entries)
	}
	if args[0] != "apply" {
		return fmt.Errorf("unknown component command")
	}
	var change componentRequest
	if err := readJSON(f.Arg(1), &change); err != nil {
		return err
	}
	p, err := e.buildPlan(ctx, request{Harness: id, Action: "manage", Component: &change})
	if err != nil {
		return err
	}
	b := &batchPlan{ID: randomID(), Plans: []*plan{p}}
	b.Digest = valueDigest(b.Plans)
	if err = printBatch(out, b); err != nil {
		return err
	}
	if *preview {
		return nil
	}
	if err = confirmCLI(in, out, b.ID, *yes); err != nil {
		return err
	}
	return e.executeBatch(ctx, b, b.ID, func(s string) { fmt.Fprintln(out, s) })
}
