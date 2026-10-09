package manager

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/zaRizk7/harness-ctl/internal/setup"
)

// setupCLI previews private setup or verifies a local release binary. Metadata
// initialization retains existing user edits. Publication needs explicit approval.
func (e *engine) setupCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	f := flag.NewFlagSet("setup", flag.ContinueOnError)
	f.SetOutput(out)
	source := f.String("binary", "", "verified downloaded executable")
	checksum := f.String("sha256", "", "expected executable SHA-256 from the release publisher")
	prefix := f.String("prefix", filepath.Join(e.cfg.Root, "app"), "private executable directory")
	link := f.String("link-dir", "", "optional directory for a manager symlink")
	shell := f.String("shell-file", "", "explicit POSIX shell startup file to append reviewed PATH setup")
	headless := f.Bool("headless", false, "use text preview and approval instead of setup TUI")
	yes := f.Bool("yes", false, "approve the displayed setup preview")
	preview := f.Bool("preview", false, "print setup preview without publication")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected setup arguments")
	}
	catalogData, err := marshalJSONIndent(e.cfg.Harnesses, "", "  ")
	if err != nil {
		return err
	}
	cfg := e.cfg
	cfg.CatalogFile = filepath.Join(cfg.Root, "catalog.json")
	configData, err := marshalJSONIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	o := setup.Options{Home: cfg.Home, Root: cfg.Root, Source: *source, SHA256: *checksum, Prefix: *prefix, LinkDir: *link, MaxBytes: cfg.PackageBytes, Files: map[string][]byte{"catalog.json": append(catalogData, '\n'), "config.json": append(configData, '\n')}}
	o.ShellFile = *shell
	o.BinDirs = []string{cfg.BinDir}
	if *source != "" {
		if *link != "" {
			o.BinDirs = append(o.BinDirs, *link)
		} else {
			o.BinDirs = append(o.BinDirs, *prefix)
		}
	}
	p, err := setup.Build(o)
	if err != nil {
		return err
	}
	if err = outputJSON(out, struct {
		ID    string
		Paths []string
	}{p.ID(), p.Paths}); err != nil {
		return err
	}
	if *preview {
		return nil
	}
	apply := func(p *setup.Plan) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		unlock, err := e.lock()
		if err != nil {
			return err
		}
		defer unlock()
		if err = e.ensureNoPending(); err != nil {
			return err
		}
		return setup.Apply(p, p.ID())
	}
	if !*headless && !*yes {
		return runSetupTUI(o, apply)
	}
	if err = confirmCLI(in, out, p.ID(), *yes); err != nil {
		return err
	}
	if err = apply(p); err != nil {
		return err
	}
	if p.Binary != "" {
		fmt.Fprintf(out, "Installed %s\n", p.Binary)
	}
	fmt.Fprintf(out, "Configuration: %s\nPATH: %s\n", filepath.Join(cfg.Root, "config.json"), cfg.BinDir)
	return nil
}
