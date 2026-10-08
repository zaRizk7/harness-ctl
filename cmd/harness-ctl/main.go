// Command harness-ctl manages local coding harnesses through an interactive TUI.
package main

import (
	"github.com/zaRizk7/harness-ctl/internal/manager"
	"os"
)

func main() { os.Exit(manager.Main(os.Args[1:])) }
