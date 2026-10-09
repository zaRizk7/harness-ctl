package manager

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/zaRizk7/harness-ctl/internal/library"
)

// libraryCLI manages reusable records and previews approved compatible application.
// Metadata output omits reusable configuration, which may contain credentials.
func (e *engine) libraryCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("library list|set FILE|enable ID|disable ID|remove ID|apply ID HARNESS...")
	}
	f := flag.NewFlagSet("library", flag.ContinueOnError)
	f.SetOutput(out)
	yes := f.Bool("yes", false, "approve displayed change")
	preview := f.Bool("preview", false, "preview only")
	owners := f.String("owners", "", "affected owner selection")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if args[0] == "list" {
		if f.NArg() != 0 {
			return fmt.Errorf("list takes no arguments")
		}
		items, err := e.loadLibrary()
		if err != nil {
			return err
		}
		return outputJSON(out, libraryViews(items))
	}
	if f.NArg() < 1 {
		return fmt.Errorf("library command requires an entry or request file")
	}
	if args[0] == "apply" {
		if f.NArg() < 2 {
			return fmt.Errorf("choose compatible harnesses")
		}
		var selectedOwners []string
		if *owners != "" {
			selectedOwners = strings.Split(*owners, ",")
		}
		p, err := e.buildLibraryApply(ctx, f.Arg(0), f.Args()[1:], selectedOwners)
		if err != nil {
			return err
		}
		if err = printBatch(out, p.Batch); err != nil {
			return err
		}
		if *preview {
			return nil
		}
		if err = confirmCLI(in, out, p.Batch.ID, *yes); err != nil {
			return err
		}
		return e.executeLibraryApply(ctx, p, p.Batch.ID, func(s string) { fmt.Fprintln(out, s) })
	}
	if f.NArg() != 1 {
		return fmt.Errorf("library record command takes one argument")
	}
	path, err := e.libraryPath()
	if err != nil {
		return err
	}
	before, err := fingerprint(path)
	if err != nil {
		return err
	}
	switch args[0] {
	case "set":
		var item library.Item
		if err = readJSON(f.Arg(0), &item); err != nil {
			return err
		}
		item, err = library.Capture(item, e.cfg.MetadataBytes)
		if err != nil {
			return err
		}
		if err = e.validateLibraryItem(item); err != nil {
			return err
		}
		if err = outputJSON(out, libraryViews([]library.Item{item})); err != nil {
			return err
		}
		if *preview {
			return nil
		}
		if err = confirmCLI(in, out, "save-"+item.ID, *yes); err != nil {
			return err
		}
		return e.saveLibraryItem(item, before)
	case "enable", "disable", "remove":
		if _, err = fmt.Fprintf(out, "Library %s: %s (target copies stay unchanged)\n", f.Arg(0), args[0]); err != nil {
			return err
		}
		if *preview {
			return nil
		}
		if err = confirmCLI(in, out, args[0]+"-"+f.Arg(0), *yes); err != nil {
			return err
		}
		return e.changeLibraryItem(f.Arg(0), args[0], before)
	}
	return fmt.Errorf("unknown library command")
}
