package main

import (
	"fmt"
	"os"
)

// Version information (set by build flags)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("prov version %s\n", Version)
		fmt.Printf("commit: %s\n", GitCommit)
		fmt.Printf("built: %s\n", BuildDate)
		os.Exit(0)
	}

	// TODO: Implement CLI commands
	// Phase 0: init, daemon start/stop, list, show, search
	// Phase 1+: session, stats, blame, etc.

	fmt.Println("AI Provenance - Phase 0 Development")
	fmt.Println("Usage: prov <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  version       Show version information")
	fmt.Println("  init          Initialize provenance storage (TODO)")
	fmt.Println("  daemon        Control background daemon (TODO)")
	fmt.Println("  list          List recent prompts (TODO)")
	fmt.Println("  show          Show prompt details (TODO)")
	fmt.Println("  search        Search prompts (TODO)")
	fmt.Println()
	fmt.Println("Run 'prov <command> --help' for more information")
}
