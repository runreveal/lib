// Command example demonstrates the github.com/runreveal/lib/cli framework,
// including await integration for long-running services.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/runreveal/lib/await"
	"github.com/runreveal/lib/cli"
)

// ---------------------------------------------------------------------------
// Globals: shared flags + resources via Configure/Validate/Close
// ---------------------------------------------------------------------------

// Globals holds flags and resources shared across all commands.
type Globals struct {
	Verbose bool   `cli:"verbose,v" usage:"enable verbose output"`
	Config  string `cli:"config,c"  usage:"config file path"      default:"config.json"`
}

// ---------------------------------------------------------------------------
// serve: a long-running HTTP server managed by await
// ---------------------------------------------------------------------------

type ServeCmd struct {
	Addr    string        `cli:"addr,a"    usage:"listen address"  default:":8080"`
	Timeout time.Duration `cli:"timeout,t" usage:"request timeout" default:"30s"`
	DB      DBConfig
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

// Run starts the HTTP server using await for graceful shutdown.
// The ctx passed by the cli framework is cancelled on SIGINT/SIGTERM
// when the root command is run, but await.WithSignals gives you the
// same behavior with named sub-runners and a configurable stop timeout.
func (s *ServeCmd) Run(ctx context.Context, args []string) error {
	g := cli.GlobalsFromContext[Globals](ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	server := &http.Server{
		Addr:         s.Addr,
		Handler:      mux,
		ReadTimeout:  s.Timeout,
		WriteTimeout: s.Timeout,
	}

	if g != nil && g.Verbose {
		fmt.Printf("starting server on %s\n", s.Addr)
		if s.DB.DSN != "" {
			fmt.Printf("database: %s\n", s.DB.DSN)
		}
	}

	// await manages graceful shutdown: on SIGINT/SIGTERM it cancels
	// the context, ListenAndServe calls server.Shutdown, and await
	// waits up to the stop timeout for in-flight requests to drain.
	w := await.New(await.WithSignals)
	w.AddNamed(await.ListenAndServe(server), "http")
	return w.Run(ctx)
}

// ---------------------------------------------------------------------------
// daemon: run multiple services concurrently with await
// ---------------------------------------------------------------------------

type DaemonCmd struct {
	APIAddr    string `cli:"api-addr"    usage:"API listen address"     default:":8080"`
	MetricAddr string `cli:"metric-addr" usage:"metrics listen address" default:":9090"`
}

func (d *DaemonCmd) Validate() error {
	if d.APIAddr == "" || d.MetricAddr == "" {
		return fmt.Errorf("both --api-addr and --metric-addr are required")
	}
	return nil
}

// Run starts multiple services under a single await runner.
// If any service exits with an error, await cancels the others
// and waits for them to shut down cleanly.
func (d *DaemonCmd) Run(ctx context.Context, args []string) error {
	apiServer := &http.Server{
		Addr:    d.APIAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "api") }),
	}
	metricServer := &http.Server{
		Addr:    d.MetricAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "metrics") }),
	}

	w := await.New(await.WithSignals, await.WithStopTimeout(15*time.Second))
	w.AddNamed(await.ListenAndServe(apiServer), "api")
	w.AddNamed(await.ListenAndServe(metricServer), "metrics")

	fmt.Printf("daemon: api=%s metrics=%s\n", d.APIAddr, d.MetricAddr)
	return w.Run(ctx)
}

// ---------------------------------------------------------------------------
// worker: a background job that respects context cancellation
// ---------------------------------------------------------------------------

type WorkerCmd struct {
	Interval time.Duration `cli:"interval,i" usage:"poll interval" default:"10s"`
}

// Run demonstrates a polling worker that exits cleanly on SIGINT/SIGTERM.
// For a single long-running goroutine, you don't need await — just
// select on ctx.Done().
func (w *WorkerCmd) Run(ctx context.Context, args []string) error {
	fmt.Printf("worker: polling every %s\n", w.Interval)
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("worker: shutting down")
			return nil
		case <-ticker.C:
			fmt.Println("worker: tick")
		}
	}
}

// ---------------------------------------------------------------------------
// migrate: a one-shot command (no await needed)
// ---------------------------------------------------------------------------

type MigrateCmd struct {
	DryRun bool   `cli:"dry-run" usage:"print migrations without running"`
	DB     string `cli:"db"      usage:"database name"                    default:"prod"`
}

func (m *MigrateCmd) Run(ctx context.Context, args []string) error {
	if m.DryRun {
		fmt.Printf("[dry-run] would migrate database: %s\n", m.DB)
	} else {
		fmt.Printf("migrating database: %s\n", m.DB)
	}
	return nil
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	globals := &Globals{}
	serveCmd := &ServeCmd{}

	app := cli.New("example", "Example CLI demonstrating cli + await",
		cli.WithVersion("1.0.0"),
		cli.WithGlobals(globals),
		cli.WithConfigFlag("config"),
	)

	app.AddCommand(
		// Long-running server with await + graceful shutdown
		cli.Command("serve", "Start the HTTP server", serveCmd,
			cli.ConfigAt("database", &serveCmd.DB),
		),

		// Multiple services under one await runner
		cli.Command("daemon", "Run all services", &DaemonCmd{}),

		// Background worker using context cancellation
		cli.Command("worker", "Run the background worker", &WorkerCmd{}),

		// One-shot commands don't need await
		cli.Group("admin", "Administrative commands",
			cli.Command("migrate", "Run database migrations", &MigrateCmd{},
				cli.WithArgs(cli.NoArgs),
			),
		),
	)

	os.Exit(app.Run(context.Background(), os.Args[1:]))
}
