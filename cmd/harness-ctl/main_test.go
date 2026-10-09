package main

import (
	"os"
	"testing"
)

func TestMainForwardsProcessStatus(t *testing.T) {
	oldArgs, oldExit := os.Args, exit
	defer func() { os.Args = oldArgs; exit = oldExit }()
	os.Args = []string{"harness-ctl", "version"}
	called := false
	exit = func(status int) {
		called = true
		if status != 0 {
			t.Fatal(status)
		}
	}
	main()
	if !called {
		t.Fatal("process status not forwarded")
	}
}
