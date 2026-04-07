# cli

A struct-driven CLI framework for Go with a Configure-Validate-Run lifecycle.

Designed as a simpler alternative to Cobra+Viper, with native integration with
[`loader`](../loader) (config file loading with HuJSON + env var replacement)
and [`await`](../await) (goroutine lifecycle and graceful shutdown).

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/runreveal/lib/cli"
)

type ServeCmd struct {
    Addr string `cli:"addr,a" usage:"listen address" default:":8080"`
}

func (s *ServeCmd) Run(ctx context.Context, args []string) error {
    fmt.Printf("serving on %s\n", s.Addr)
    return nil
}

func main() {
    app := cli.New("myapp", "My application")
    app.AddCommand(cli.Command("serve", "Start the server", &ServeCmd{}))
    os.Exit(app.Run(context.Background(), os.Args[1:]))
}
```

## Features

### Struct Tags

Command structs use tags to define flags and config bindings:

```go
type ServeCmd struct {
    Addr    string        `cli:"addr,a"    usage:"listen address"  default:":8080"`
    Timeout time.Duration `cli:"timeout,t" usage:"request timeout" default:"30s"`
    DB      DBConfig      `config:"database"` // loaded from config file
}
```

| Tag | Purpose |
|---|---|
| `cli:"name,alias"` | Flag name and optional single-char alias |
| `cli:"-"` | Skip this field |
| `usage:"text"` | Help text |
| `default:"value"` | Default value (parsed to field type) |
| `config:"key"` | JSON path in config file to unmarshal into this field |

Supported types: `string`, `bool`, `int`, `int64`, `uint`, `uint64`, `float64`,
`time.Duration`, `[]string`, pointer variants, and `encoding.TextUnmarshaler`.

### Global Flags

Define shared flags once at the app level instead of embedding in every command:

```go
type Globals struct {
    Verbose bool   `cli:"verbose,v" usage:"enable verbose output"`
    Config  string `cli:"config,c"  usage:"config file" default:"config.json"`
}

globals := &Globals{}
app := cli.New("myapp", "desc",
    cli.WithGlobals(globals),
    cli.WithConfigFlag("config"),
)

// Access in any handler:
func (s *ServeCmd) Run(ctx context.Context, args []string) error {
    g := cli.GlobalsFromContext[Globals](ctx)
    if g.Verbose { ... }
}
```

### Configure-Validate-Run Lifecycle

Globals and handlers can implement optional lifecycle interfaces:

```go
// Configurer is called after config loading to initialize resources.
type Configurer interface {
    Configure() error
}

// Validator is called after Configure to check readiness.
type Validator interface {
    Validate() error
}
```

The framework also calls `io.Closer` on globals after the command exits.

**Lifecycle order:**
1. Parse flags (globals + handler)
2. Load config file into struct fields
3. Globals: `Configure()` -> `Validate()` -> defer `Close()`
4. Handler: `Configure()` -> `Validate()`
5. Middleware -> `handler.Run(ctx, args)`
6. Globals `Close()`

### Config File Loading

One config file for the whole app. Commands declare which sections they need
using `config:"key"` struct tags:

```go
type ServeCmd struct {
    DB DBConfig `config:"database"`
}
```

Config files are processed through `loader.LoadConfig`, which supports HuJSON
(comments, trailing commas) and `$ENV_VAR` replacement in string values.

**Precedence:** explicit CLI flag > config file > default tag > zero value

### Commands and Groups

```go
app.AddCommand(
    // Command with handler
    cli.Command("serve", "Start the server", &ServeCmd{}),

    // Command with long-form help text shown by --help
    cli.Command("up", "Launch a forge pod", &UpCmd{},
        cli.WithLong(`Launch a new forge pod for the given branch.

If the branch doesn't exist, create it first. The pod runs Claude Code
in headless mode with the given prompt.`),
    ),

    // Command with handler AND subcommands
    cli.Command("admin", "Admin tools", &AdminCmd{},
        cli.Command("migrate", "Run migrations", &MigrateCmd{}),
    ),

    // Group (no handler, prints help when invoked directly)
    cli.Group("db", "Database commands",
        cli.WithLong("Manage database migrations and connections."),
        cli.Command("migrate", "Run migrations", &MigrateCmd{}),
        cli.Command("seed", "Seed data", &SeedCmd{}),
    ),
)
```

The short description is always used in parent command listings. The long text
appears indented below the title line when the user runs `myapp <command> --help`:

```
myapp up - Launch a forge pod

  Launch a new forge pod for the given branch.

  If the branch doesn't exist, create it first. The pod runs Claude Code
  in headless mode with the given prompt.

Usage:
  myapp up [flags]

Flags:
  ...
```

### Command Registry

Allow commands to register themselves from outside the main package, enabling
build-tag-based inclusion of optional commands:

```go
// cmd/myapp/debug/debug.go
//go:build debug

package debug

import (
    "context"
    "github.com/runreveal/lib/cli"
)

type DebugCmd struct{}

func (d *DebugCmd) Run(ctx context.Context, args []string) error { ... }

func init() {
    cli.Register(cli.Command("debug", "Debug tools", &DebugCmd{}))
}
```

```go
// cmd/myapp/main.go
package main

import (
    "github.com/runreveal/lib/cli"
    _ "myapp/cmd/myapp/debug" // only included with -tags debug
)

func main() {
    app := cli.New("myapp", "My app")
    app.AddCommand(cli.Registered()...) // commands from init() calls
    app.AddCommand(                      // explicit commands
        cli.Command("serve", "...", &ServeCmd{}),
    )
    os.Exit(app.Run(context.Background(), os.Args[1:]))
}
```

`Register` is safe to call from `init()` and accumulates across multiple calls.
`Registered` returns a copy of the registered nodes.

### Middleware

```go
app := cli.New("myapp", "desc",
    cli.WithMiddleware(func(ctx context.Context, info cli.CommandInfo, next func(context.Context) error) error {
        slog.Info("running", "command", info.Name)
        return next(ctx)
    }),
)
```

### Args Validation

```go
cli.Command("get", "Get a resource", &GetCmd{}, cli.WithArgs(cli.ExactArgs(1)))
cli.Command("run", "Run a task", &RunCmd{}, cli.WithArgs(cli.NoArgs))
cli.Command("ping", "Ping hosts", &PingCmd{}, cli.WithArgs(cli.MinArgs(1)))
```

### Await Integration

For long-running services, use `await` in your handler's `Run` method:

```go
func (s *ServeCmd) Run(ctx context.Context, args []string) error {
    server := &http.Server{Addr: s.Addr, Handler: mux}

    w := await.New(await.WithSignals)
    w.AddNamed(await.ListenAndServe(server), "http")
    return w.Run(ctx)
}
```

For multiple services under one process:

```go
func (d *DaemonCmd) Run(ctx context.Context, args []string) error {
    w := await.New(await.WithSignals, await.WithStopTimeout(15*time.Second))
    w.AddNamed(await.ListenAndServe(apiServer), "api")
    w.AddNamed(await.ListenAndServe(metricServer), "metrics")
    return w.Run(ctx)
}
```

### Shell Completion

```bash
# Generate completion script
eval "$(myapp completion bash)"   # or zsh, fish
```

Handlers can provide custom completions for positional args:

```go
func (g *GetCmd) Complete(ctx context.Context, args []string) []cli.Completion {
    return []cli.Completion{
        {Value: "pods", Description: "list pods"},
        {Value: "services", Description: "list services"},
    }
}
```

### Default Config

Ship a reference config with your binary using `go:embed`:

```go
//go:embed config.json
var defaultConfig []byte

app := cli.New("myapp", "desc",
    cli.WithDefaultConfig(defaultConfig),
)
```

```bash
myapp defcon > config.json   # dump the default config
```

The command name is `defcon` by default, overridable with `WithDefaultConfigCommand`.

### Built-in Flags

| Flag | Behavior |
|---|---|
| `-h`, `--help` | Print help for the app or command |
| `--version` | Print version (requires `WithVersion`) |

## See Also

- [`await`](../await) — goroutine lifecycle management
- [`loader`](../loader) — polymorphic config loading
- [`cli/example`](./example) — complete working example
