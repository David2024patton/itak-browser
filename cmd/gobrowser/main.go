// gobrowser is the iTaK Browser CLI binary entry point.
//
// What: Parses commands and dispatches to the persistent daemon.
// Why:  A single binary ships the full iTaK Browser. Users call 'gobrowser'
//       directly; agents call it via shell with --json for machine-readable output.
// How:  Cobra root command tree built in pkg/cli. 'daemon start' invocation
//       also starts the daemon server in the same process.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/David2024patton/iTaKBrowser/pkg/cli"
	"github.com/David2024patton/iTaKBrowser/pkg/daemon"
)

func main() {
	// Special case: if the first argument is "daemon-internal",
	// this process IS the daemon (spawned as a subprocess by the CLI).
	if len(os.Args) > 1 && os.Args[1] == "daemon-internal" {
		port := daemon.DefaultPort
		if len(os.Args) > 2 {
			if p, err := strconv.Atoi(os.Args[2]); err == nil {
				port = p
			}
		}
		runDaemon(port)
		return
	}

	root := cli.NewRootCmd()

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// runDaemon starts the daemon server (blocking).
func runDaemon(port int) {
	d := daemon.New(port, nil)
	fmt.Printf("iTaK Browser daemon starting on port %d...\n", port)
	if err := d.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "daemon error: %v\n", err)
		os.Exit(1)
	}
}
