// Command harness-ctl manages local coding harnesses through a TUI or CLI.
package main

import (
	"github.com/zaRizk7/harness-ctl/internal/manager"
	"os"
)

var exit = os.Exit

// main forwards process arguments and uses the command exit status.
func main() { exit(manager.Main(os.Args[1:])) }
