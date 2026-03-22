package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type globalOptions struct {
	InputPath  string
	APIKey     string
	ConfigPath string
	Format     string
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	globals, rest, err := parseGlobalFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printRootUsage(os.Stdout)
			return nil
		}
		return err
	}
	if len(rest) == 0 {
		printRootUsage(os.Stdout)
		return nil
	}

	cfg, err := loadRuntimeConfig(globals)
	if err != nil {
		return err
	}

	switch rest[0] {
	case "help":
		printRootUsage(os.Stdout)
		return nil
	case "init":
		return runInit(ctx, cfg, rest[1:])
	case "doctor":
		return runDoctor(ctx, cfg, rest[1:])
	case "analyze":
		return runAnalyze(ctx, cfg, rest[1:])
	case "exercises":
		return runExercises(ctx, cfg, rest[1:])
	case "routines":
		return runRoutines(ctx, cfg, rest[1:])
	case "workouts":
		return runWorkouts(ctx, cfg, rest[1:])
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func parseGlobalFlags(args []string) (globalOptions, []string, error) {
	var opts globalOptions
	fs := flag.NewFlagSet("strong2hevy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.InputPath, "input", "", "Path to Strong CSV export")
	fs.StringVar(&opts.APIKey, "api-key", "", "Hevy API key")
	fs.StringVar(&opts.ConfigPath, "config", "", "Path to config file")
	fs.StringVar(&opts.Format, "format", "", "Output format: table or json")
	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	return opts, fs.Args(), nil
}

func printRootUsage(w io.Writer) {
	usage := []string{
		"Usage: strong2hevy [global flags] <command> [args]",
		"",
		"Global flags:",
		"  --input <path>      Path to Strong CSV export",
		"  --api-key <key>     Hevy API key (or HEVY_API_KEY)",
		"  --config <path>     Config file path (default .strong2hevy/config.yaml)",
		"  --format <format>   Output format: table or json",
		"",
		"Commands:",
		"  init                        Create .strong2hevy/config.yaml",
		"  doctor                      Validate local config and Hevy connectivity",
		"  analyze                     Summarize the Strong CSV",
		"  exercises search <query>    Search Hevy exercise templates",
		"  exercises resolve           Build or update exercise mapping file",
		"  exercises review            Interactively review unresolved exercise mappings",
		"  routines plan               Build routine candidates from repeated workouts",
		"  routines apply              Create or update Hevy routines",
		"  workouts import            Import completed workouts into Hevy",
	}
	fmt.Fprintln(w, strings.Join(usage, "\n"))
}
