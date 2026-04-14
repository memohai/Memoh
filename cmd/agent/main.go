package main

import (
	"fmt"
	"os"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		runServe()
	case "migrate":
		runMigrate(os.Args[2:])
	case "version":
		if err := runVersion(); err != nil {
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Usage: memoh-server <command>\n\nCommands:\n  serve     Start the server (default)\n  migrate   Run database migrations (up|down|version|force)\n  version   Print version information\n")
		os.Exit(1)
	}
}

func runMigrate(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: memoh-server migrate <up|down|version|force N>\n")
		os.Exit(1)
	}
	if err := runMigrateCommand(args); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}
