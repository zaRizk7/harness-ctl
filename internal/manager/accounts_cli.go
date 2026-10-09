package manager

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// accountsCLI manages the independent encrypted account vault and reads provider
// reports. Keys/plan changes and invoices open native pages rather than sending
// financial mutations or provisioning remote accounts.
func (e *engine) accountsCLI(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("accounts list|set FILE|remove ID|enable ID|disable ID|monitor [--watch] [ID]|open ID billing|usage|keys|subscription")
	}
	if args[0] == "set" {
		if len(args) != 2 {
			return fmt.Errorf("accounts set requires a JSON file")
		}
		var a account
		if err := readJSON(args[1], &a); err != nil {
			return err
		}
		return e.saveAccount(a)
	}
	items, before, err := e.readAccountState()
	if err != nil {
		return err
	}
	if args[0] == "list" {
		return outputJSON(out, accountViews(items))
	}
	if args[0] == "monitor" {
		f := flag.NewFlagSet("monitor", flag.ContinueOnError)
		f.SetOutput(out)
		watch := f.Bool("watch", false, "refresh until interrupted")
		seconds := f.Int("refresh", e.cfg.RefreshSeconds, "refresh interval in seconds")
		if err = f.Parse(args[1:]); err != nil {
			return err
		}
		if *seconds < 1 || f.NArg() > 1 {
			return fmt.Errorf("monitor requires a positive refresh and optional account ID")
		}
		for {
			if err = ctx.Err(); err != nil {
				return err
			}
			if *watch {
				items, err = e.loadAccounts()
				if err != nil {
					return err
				}
			}
			selected := items
			if f.NArg() == 1 {
				selected = nil
				for _, a := range items {
					if a.ID == f.Arg(0) {
						selected = append(selected, a)
					}
				}
				if len(selected) == 0 {
					return fmt.Errorf("account does not exist")
				}
			}
			var metrics []accountMetric
			for _, a := range selected {
				if a.Enabled {
					metrics = append(metrics, e.monitorAccount(ctx, a, time.Now()))
				}
			}
			if err = outputJSON(out, metrics); err != nil {
				return err
			}
			if !*watch {
				return nil
			}
			timer := time.NewTimer(time.Duration(*seconds) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	if len(args) < 2 {
		return fmt.Errorf("account command requires an ID")
	}
	var chosen account
	for _, a := range items {
		if a.ID == args[1] {
			chosen = a
		}
	}
	if chosen.ID == "" {
		return fmt.Errorf("account does not exist")
	}
	switch args[0] {
	case "remove":
		if err = confirmCLI(in, out, "remove-"+chosen.ID, false); err != nil {
			return err
		}
		return e.removeAccountChecked(chosen.ID, before)
	case "enable", "disable":
		return e.setAccountEnabled(chosen.ID, args[0] == "enable", before)
	case "open":
		if len(args) != 3 {
			return fmt.Errorf("choose billing, usage, keys or subscription")
		}
		// readAccountState validated each account's provider against this catalog.
		p, _ := e.providerFor(chosen.Provider)
		link := ""
		switch args[2] {
		case "billing":
			link = p.BillingURL
		case "usage":
			link = p.UsageURL
		case "keys":
			link = p.KeysURL
		case "subscription":
			link = p.SubscriptionURL
		default:
			return fmt.Errorf("unknown account page")
		}
		if link == "" {
			return fmt.Errorf("provider has no configured %s page", args[2])
		}
		fmt.Fprintln(out, link)
		_, err = e.run.Run(ctx, command{Path: "open", Args: []string{link}, Description: "Open native account page"})
		return err
	}
	return fmt.Errorf("unknown account action %q", strings.Join(args, " "))
}
