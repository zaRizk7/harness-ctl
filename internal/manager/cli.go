package manager

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var version = "dev"

// Main runs the harness-ctl command and returns its process exit status.
func Main(args []string) int {
	flags := flag.NewFlagSet("harness-ctl", flag.ContinueOnError)
	configFile := flags.String("config", "", "JSON configuration path")
	root := flags.String("root", "", "override manager storage directory")
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "harness-ctl [--config path] [--root path] [tui|list|catalog|config|version]\n\nLifecycle operations require an interactive preview and confirmation.\nGo is user-managed. No toolchain installation is performed.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	action := "tui"
	if flags.NArg() > 0 {
		action = flags.Arg(0)
	}
	if action == "version" {
		fmt.Println("harness-ctl", version)
		return 0
	}
	if runtime.GOOS != "darwin" {
		fmt.Fprintln(os.Stderr, "harness-ctl v1 supports macOS only")
		return 1
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfg := defaultConfig(home)
	if *configFile != "" {
		if err = readJSON(*configFile, &cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if *root != "" {
		cfg.Root = filepath.Clean(*root)
		cfg.BinDir = filepath.Join(cfg.Root, "bin")
	}
	if err = cfg.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if action == "config" {
		data, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(data))
		return 0
	}
	if action == "catalog" {
		for _, s := range catalog {
			fmt.Printf("%-14s %s\n", s.ID, s.Name)
		}
		return 0
	}
	e, err := newEngine(cfg, systemRunner{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	switch action {
	case "tui":
		err = runTUI(e)
	case "list":
		var installs []installation
		installs, err = e.discover(context.Background())
		if err == nil {
			data, _ := json.MarshalIndent(installs, "", "  ")
			fmt.Println(string(data))
		}
	default:
		flags.Usage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
