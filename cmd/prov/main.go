package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Version information (set by build flags)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version":
		cmdVersion()
	case "daemon":
		cmdDaemon()
	case "list":
		cmdList()
	case "show":
		cmdShow()
	case "search":
		cmdSearch()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("AI Provenance - Track AI-assisted code changes")
	fmt.Println()
	fmt.Println("Usage: prov <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  version              Show version information")
	fmt.Println("  daemon start         Start background daemon")
	fmt.Println("  daemon stop          Stop background daemon")
	fmt.Println("  daemon status        Check daemon status")
	fmt.Println("  list                 List recent prompts")
	fmt.Println("  show <id>            Show prompt details")
	fmt.Println("  search <query>       Search prompts")
	fmt.Println()
	fmt.Println("Run 'prov <command> --help' for more information")
}

func cmdVersion() {
	fmt.Printf("prov version %s\n", Version)
	fmt.Printf("commit: %s\n", GitCommit)
	fmt.Printf("built: %s\n", BuildDate)
}

func cmdDaemon() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: prov daemon <start|stop|status>")
		os.Exit(1)
	}

	subcommand := os.Args[2]

	switch subcommand {
	case "start":
		daemonStart()
	case "stop":
		daemonStop()
	case "status":
		daemonStatus()
	case "_run":
		// Hidden subcommand used internally by daemon start
		daemonRun()
	default:
		fmt.Fprintf(os.Stderr, "Unknown daemon command: %s\n", subcommand)
		fmt.Fprintln(os.Stderr, "Usage: prov daemon <start|stop|status>")
		os.Exit(1)
	}
}

func cmdList() {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	limit := fs.Int("limit", 10, "Number of events to show")
	fs.Parse(os.Args[2:]) //nolint:errcheck

	if err := listEvents(*limit); err != nil {
		fmt.Fprintf(os.Stderr, "Error listing events: %v\n", err)
		os.Exit(1)
	}
}

func cmdShow() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: prov show <event-id>")
		os.Exit(1)
	}

	eventID := os.Args[2]

	if err := showEvent(eventID); err != nil {
		fmt.Fprintf(os.Stderr, "Error showing event: %v\n", err)
		os.Exit(1)
	}
}

func cmdSearch() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: prov search <query>")
		os.Exit(1)
	}

	query := strings.Join(os.Args[2:], " ")

	if err := searchEvents(query); err != nil {
		fmt.Fprintf(os.Stderr, "Error searching events: %v\n", err)
		os.Exit(1)
	}
}

// getProvenanceHome returns the AI_PROVENANCE_HOME directory
func getProvenanceHome() string {
	if home := os.Getenv("AI_PROVENANCE_HOME"); home != "" {
		return home
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	return filepath.Join(homeDir, ".ai-provenance")
}

// getSocketPath returns the path to the daemon socket
func getSocketPath() string {
	return filepath.Join(getProvenanceHome(), "daemon.sock")
}

// getDBPath returns the path to the database
func getDBPath() string {
	return filepath.Join(getProvenanceHome(), "db.sqlite")
}
