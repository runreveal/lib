// Command example demonstrates the github.com/runreveal/lib/cli framework.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/runreveal/lib/cli"
)

// Globals are shared flags embedded into every command.
type Globals struct {
	Verbose bool   `cli:"verbose,v" usage:"enable verbose output"`
	Config  string `cli:"config,c"  usage:"config file path" default:"config.json"`
}

// ServeCmd is the handler for the "serve" subcommand.
type ServeCmd struct {
	Globals
	Addr    string        `cli:"addr,a"    usage:"listen address"     default:":8080"`
	Timeout time.Duration `cli:"timeout,t" usage:"request timeout"    default:"30s"`

	// DB is loaded from the config file's "database" section.
	DB DBConfig `config:"database"`
}

type DBConfig struct {
	DSN string `json:"dsn"`
}

func (s *ServeCmd) Validate() error {
	if s.Addr == "" {
		return fmt.Errorf("--addr must not be empty")
	}
	return nil
}

func (s *ServeCmd) Run(ctx context.Context, args []string) error {
	if s.Verbose {
		fmt.Printf("verbose mode enabled\n")
		fmt.Printf("config file: %s\n", s.Config)
		if s.DB.DSN != "" {
			fmt.Printf("database DSN: %s\n", s.DB.DSN)
		}
	}
	fmt.Printf("serving on %s (timeout: %s)\n", s.Addr, s.Timeout)
	return nil
}

// MigrateCmd is in the "admin" group.
type MigrateCmd struct {
	Globals
	DryRun bool   `cli:"dry-run" usage:"print migrations without running"`
	DB     string `cli:"db"      usage:"database name" default:"prod"`
}

func (m *MigrateCmd) Run(ctx context.Context, args []string) error {
	if m.DryRun {
		fmt.Printf("[dry-run] would migrate database: %s\n", m.DB)
	} else {
		fmt.Printf("migrating database: %s\n", m.DB)
	}
	return nil
}

// PingCmd demonstrates positional args.
type PingCmd struct {
	Globals
	Count int `cli:"count,n" usage:"number of pings" default:"3"`
}

func (p *PingCmd) Run(ctx context.Context, args []string) error {
	hosts := args
	if len(hosts) == 0 {
		hosts = []string{"localhost"}
	}
	for _, host := range hosts {
		for i := 0; i < p.Count; i++ {
			fmt.Printf("ping #%d -> %s\n", i+1, host)
		}
	}
	return nil
}

func main() {
	// Middleware: log every command execution
	loggingMW := func(ctx context.Context, info cli.CommandInfo, next func(context.Context) error) error {
		fmt.Printf("[log] running command: %s\n", info.Name)
		err := next(ctx)
		if err != nil {
			fmt.Printf("[log] command failed: %v\n", err)
		}
		return err
	}

	app := cli.New("example", "Example CLI demonstrating the cli framework",
		cli.WithVersion("1.0.0"),
		cli.WithConfigFlag("config"),
		cli.WithMiddleware(loggingMW),
	)

	app.AddCommand(
		cli.Command("serve", "Start the HTTP server", &ServeCmd{}),
		cli.CommandWithOptions("ping", "Ping one or more hosts", &PingCmd{},
			[]cli.CmdOption{cli.WithArgs(cli.MinArgs(0))},
		),
		cli.Group("admin", "Administrative commands",
			cli.CommandWithOptions("migrate", "Run database migrations", &MigrateCmd{},
				[]cli.CmdOption{cli.WithArgs(cli.NoArgs)},
			),
		),
	)

	os.Exit(app.Run(context.Background(), os.Args[1:]))
}
