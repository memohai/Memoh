package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/memohai/memoh/internal/tui/local"
)

// runSlot implements the `memoh-server slot` dev-only command group used
// by the desktop dev launcher to resolve per-keyword ports and data
// directories. It is not part of the server runtime.
func runSlot(args []string) {
	if len(args) == 0 {
		slotUsage()
		os.Exit(1)
	}
	rest := args[1:]
	switch args[0] {
	case "resolve":
		if err := slotResolve(rest, false); err != nil {
			fmt.Fprintf(os.Stderr, "slot resolve: %v\n", err)
			os.Exit(1)
		}
	case "env":
		if err := slotResolve(rest, true); err != nil {
			fmt.Fprintf(os.Stderr, "slot env: %v\n", err)
			os.Exit(1)
		}
	case "list":
		if err := slotList(); err != nil {
			fmt.Fprintf(os.Stderr, "slot list: %v\n", err)
			os.Exit(1)
		}
	default:
		slotUsage()
		os.Exit(1)
	}
}

func slotUsage() {
	fmt.Fprintf(os.Stderr, "Usage: memoh-server slot <resolve|env|list> [name]\n\n"+
		"  resolve <name>  Print slot ports + data dir as JSON (allocates on first use)\n"+
		"  env <name>      Print shell KEY=VALUE lines for `eval` (desktop dev launch)\n"+
		"  list            List registered dev slots\n")
}

func slotResolve(args []string, asEnv bool) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	slot, err := local.ResolveDevSlot(name)
	if err != nil {
		return err
	}
	if asEnv {
		fmt.Printf("MEMOH_SLOT=%s\n", slot.Name)
		fmt.Printf("MEMOH_SLOT_SERVER_PORT=%d\n", slot.ServerPort)
		fmt.Printf("MEMOH_WEB_PORT=%d\n", slot.WebPort)
		fmt.Printf("MEMOH_WEB_PROXY_TARGET=http://127.0.0.1:%d\n", slot.ServerPort)
		return nil
	}
	encoded, err := json.MarshalIndent(slot, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func slotList() error {
	slots, err := local.ListDevSlots()
	if err != nil {
		return err
	}
	for _, s := range slots {
		fmt.Printf("%-20s server=:%d web=%d  %s\n", s.Name, s.ServerPort, s.WebPort, s.DataDir)
	}
	return nil
}
