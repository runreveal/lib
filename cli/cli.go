// Package cli provides a simple, struct-driven CLI framework.
// It follows a Configure-Validate-Run lifecycle and integrates
// with github.com/runreveal/lib/loader for config file loading.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
)

// flagSetKey is the context key used to carry the *FlagSet during command execution.
type flagSetKey struct{}

// globalsKey is the context key used to carry the globals pointer during execution.
type globalsKey struct{}

// Runnable is the core interface every command handler must implement.
type Runnable interface {
	Run(ctx context.Context, args []string) error
}

// Configurer is optionally implemented by globals or handler structs.
// Called after config file loading to initialize resources (e.g. open
// database connections, create clients).
type Configurer interface {
	Configure() error
}

// Validator is optionally implemented by globals or handler structs.
// Called after Configure to check that the fully-loaded config is valid.
type Validator interface {
	Validate() error
}

// HelpExtra is optionally implemented by command handlers to append
// additional information to help output. Useful for showing available
// loader types, config file schemas, or other context that the cli
// framework can't derive from struct tags alone.
type HelpExtra interface {
	ExtraHelp() string
}

// ExitError carries a custom exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit code %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// CommandInfo is passed to middleware.
type CommandInfo struct {
	Name string   // full command path, e.g. "admin migrate"
	Args []string // positional args after flag parsing
}

// Middleware wraps command execution.
type Middleware func(ctx context.Context, info CommandInfo, next func(context.Context) error) error

// Node is a node in the command tree.
type Node interface {
	nodeName() string
	nodeDesc() string
	isGroup() bool
}

type commandNode struct {
	name     string
	desc     string
	long     string
	aliases  []string
	handler  Runnable
	children []Node
	opts     cmdOptions
}

func (c *commandNode) nodeName() string { return c.name }
func (c *commandNode) nodeDesc() string { return c.desc }
func (c *commandNode) isGroup() bool    { return false }

type groupNode struct {
	name     string
	desc     string
	long     string
	aliases  []string
	children []Node
}

func (g *groupNode) nodeName() string { return g.name }
func (g *groupNode) nodeDesc() string { return g.desc }
func (g *groupNode) isGroup() bool    { return true }

// CmdOption configures a Command or Group node.
type CmdOption func(*cmdOptions)

type cmdOptions struct {
	argsFunc ArgsFunc
	long     string
	aliases  []string
}

// WithArgs sets an args validation function on a command.
func WithArgs(f ArgsFunc) CmdOption {
	return func(o *cmdOptions) { o.argsFunc = f }
}

// WithLong sets long-form help text shown when the command is invoked with
// --help. The short description is still used in parent command listings.
func WithLong(text string) CmdOption {
	return func(o *cmdOptions) { o.long = text }
}

// Aliases sets alternate names a command or group also answers to.
// Aliases are matched during routing and completion traversal, but are not
// listed alongside canonical names in help output or completion suggestions.
// Useful for keeping old command names working after a rename.
func Aliases(names ...string) CmdOption {
	return func(o *cmdOptions) { o.aliases = append(o.aliases, names...) }
}

// parseNodeOpts processes the variadic opts accepted by Command and Group,
// separating Node children from CmdOption configurers.
func parseNodeOpts(kind, name string, opts []any) ([]Node, cmdOptions) {
	var o cmdOptions
	var children []Node
	for _, opt := range opts {
		switch v := opt.(type) {
		case Node:
			children = append(children, v)
		case CmdOption:
			v(&o)
		default:
			panic(fmt.Sprintf("cli.%s %q: unsupported option type %T", kind, name, v))
		}
	}
	return children, o
}

// Command creates a command node. Each element of opts may be a Node (child
// subcommand) or a CmdOption (behavioural option); they are distinguished by
// type at runtime.
func Command(name, desc string, handler Runnable, opts ...any) Node {
	children, o := parseNodeOpts("Command", name, opts)
	return &commandNode{name: name, desc: desc, long: o.long, aliases: o.aliases, handler: handler, children: children, opts: o}
}

// Group creates a group node that only prints help when invoked directly.
// Each element of opts may be a Node (child subcommand) or a CmdOption
// (e.g. WithLong); they are distinguished by type at runtime.
func Group(name, desc string, opts ...any) Node {
	children, o := parseNodeOpts("Group", name, opts)
	return &groupNode{name: name, desc: desc, long: o.long, aliases: o.aliases, children: children}
}

// AppOption configures an App.
type AppOption func(*App)

// WithVersion sets the application version (enables --version flag).
func WithVersion(v string) AppOption {
	return func(a *App) { a.version = v }
}

// WithMiddleware adds a middleware to the app.
func WithMiddleware(m Middleware) AppOption {
	return func(a *App) { a.middlewares = append(a.middlewares, m) }
}

// WithConfigFlag sets which flag name holds the config file path.
func WithConfigFlag(flagName string) AppOption {
	return func(a *App) { a.configFlag = flagName }
}

// WithGlobals registers a struct pointer whose cli-tagged fields become
// flags available on every command. The pointer is stored in context and
// can be retrieved with GlobalsFromContext.
//
// Global flags may appear anywhere in the argument list — before, between,
// or after command and subcommand names. For example, all of these are
// equivalent:
//
//	myapp --profile staging auth login
//	myapp auth --profile staging login
//	myapp auth login --profile staging
//
// If a handler struct defines a flag with the same name as a global flag,
// the framework panics with a clear error. Rename one of them to resolve
// the collision. Two commands on separate branches may independently use
// the same flag name without conflict.
func WithGlobals(ptr any) AppOption {
	return func(a *App) {
		a.globals = ptr
		a.globalFlags = scanGlobalFlags(ptr)
	}
}

// WithDefaultConfig registers a default configuration that can be printed
// with "myapp defcon". Typically used with go:embed to ship a
// reference config alongside the binary. The command name defaults to
// "defcon" but can be overridden with WithDefaultConfigCommand.
func WithDefaultConfig(data []byte) AppOption {
	return func(a *App) { a.defaultConfig = data }
}

// WithDefaultConfigCommand overrides the command name used to print the
// default config (default: "defcon").
func WithDefaultConfigCommand(name string) AppOption {
	return func(a *App) { a.defaultConfigCmd = name }
}

// WithOutput sets the writer for all output: help/errors (normally stderr)
// and completion/defcon (normally stdout). Useful for testing.
func WithOutput(w io.Writer) AppOption {
	return func(a *App) { a.output = w; a.stdout = w }
}

// App is the top-level CLI application.
type App struct {
	name             string
	desc             string
	version          string
	configFlag       string
	globals          any // pointer to globals struct, if set
	globalFlags      map[string]globalFlagInfo
	defaultConfig    []byte
	defaultConfigCmd string
	middlewares      []Middleware
	children         []Node
	output           io.Writer // stderr: errors, help
	stdout           io.Writer // stdout: completion, defcon
}

// New creates a new App.
func New(name, desc string, opts ...AppOption) *App {
	a := &App{
		name:   name,
		desc:   desc,
		output: os.Stderr,
		stdout: os.Stdout,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// AddCommand adds top-level command nodes to the app.
func (a *App) AddCommand(nodes ...Node) {
	a.children = append(a.children, nodes...)
}

// Run executes the CLI with the given args (typically os.Args[1:]).
// Returns an exit code.
func (a *App) Run(ctx context.Context, args []string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in command", "panic", r, "stack", string(debug.Stack()))
			exitCode = 1
		}
	}()

	code, err := a.run(ctx, args)
	if err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			if exitErr.Err != nil {
				fmt.Fprintf(a.output, "error: %s\n", exitErr.Err)
			}
			return exitErr.Code
		}
		fmt.Fprintf(a.output, "error: %s\n", err)
		return 1
	}
	return code
}

func (a *App) run(ctx context.Context, args []string) (int, error) {
	// Handle built-in commands before normal routing so they stay
	// hidden from help output and don't interfere with user commands.
	if code, handled := a.handleCompletion(args); handled {
		return code, nil
	}
	if dc := a.defconCmd(); dc != "" && len(args) >= 1 && args[0] == dc {
		return a.handleDefaultConfig(), nil
	}

	// Check for top-level --version / --help before routing
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		if a.version != "" {
			fmt.Fprintf(a.output, "%s version %s\n", a.name, a.version)
		} else {
			fmt.Fprintf(a.output, "%s (no version set)\n", a.name)
		}
		return 0, nil
	}
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		printAppHelp(a.output, a.name, a.desc, a.children, a.version, a.defconCmd())
		return 0, nil
	}

	node, rest, path := routeArgsWithPath(a.children, args, "", a.globalFlags)
	if node == nil {
		first := args[0]
		if i := skipGlobalFlags(args, a.globalFlags); i < len(args) {
			first = args[i]
		}
		fmt.Fprintf(a.output, "unknown command %q\n\n", first)
		printAppHelp(a.output, a.name, a.desc, a.children, a.version, a.defconCmd())
		return 1, nil
	}

	return a.executeNode(ctx, node, rest, path)
}

// globalFlagInfo describes a known global flag for routing.
type globalFlagInfo struct {
	isBool bool
}

// scanGlobalFlags reflects on the globals struct to build a map of flag names
// (both long and short forms) to their type info.
func scanGlobalFlags(globals any) map[string]globalFlagInfo {
	rv := reflect.ValueOf(globals)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	fields, err := scanFields(rv.Type())
	if err != nil {
		return nil
	}

	m := make(map[string]globalFlagInfo)
	for _, fi := range fields {
		if fi.flagLong == "" {
			continue
		}
		ft := fi.fieldType
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		info := globalFlagInfo{isBool: ft.Kind() == reflect.Bool}
		m["--"+fi.flagLong] = info
		if fi.flagShort != "" {
			m["-"+fi.flagShort] = info
		}
	}
	return m
}

// skipGlobalFlags returns the index of the first arg that is not a recognized
// global flag (or its value). Stops at "--".
func skipGlobalFlags(args []string, gflags map[string]globalFlagInfo) int {
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			break
		}
		if eqIdx := strings.IndexByte(arg, '='); eqIdx > 0 && strings.HasPrefix(arg, "-") {
			if _, ok := gflags[arg[:eqIdx]]; ok {
				i++
				continue
			}
			break
		}
		info, ok := gflags[arg]
		if !ok {
			break
		}
		i++
		if !info.isBool && i < len(args) {
			i++
		}
	}
	return i
}

func routeArgsWithPath(
	children []Node,
	args []string,
	prefix string,
	gflags map[string]globalFlagInfo,
) (Node, []string, string) {
	i := skipGlobalFlags(args, gflags)
	if i >= len(args) {
		return nil, args, prefix
	}

	name := args[i]
	if strings.HasPrefix(name, "-") {
		return nil, args, prefix
	}

	for _, child := range children {
		if nodeMatches(child, name) {
			canonical := child.nodeName()
			fullPath := canonical
			if prefix != "" {
				fullPath = prefix + " " + canonical
			}
			rest := make([]string, 0, len(args)-1)
			rest = append(rest, args[:i]...)
			rest = append(rest, args[i+1:]...)

			var subChildren []Node
			switch n := child.(type) {
			case *commandNode:
				subChildren = n.children
			case *groupNode:
				subChildren = n.children
			}

			if len(subChildren) > 0 {
				if sub, subRest, subPath := routeArgsWithPath(subChildren, rest, fullPath, gflags); sub != nil {
					return sub, subRest, subPath
				}
			}

			return child, rest, fullPath
		}
	}
	return nil, args, prefix
}

// nodeMatches reports whether name is the node's canonical name or one of
// its aliases.
func nodeMatches(n Node, name string) bool {
	if n.nodeName() == name {
		return true
	}
	switch v := n.(type) {
	case *commandNode:
		return slices.Contains(v.aliases, name)
	case *groupNode:
		return slices.Contains(v.aliases, name)
	}
	return false
}

func (a *App) executeNode(ctx context.Context, node Node, args []string, path string) (int, error) {
	switch n := node.(type) {
	case *groupNode:
		// Groups print help when invoked directly (no matching subcommand)
		printGroupHelp(a.output, a.name, path, n.desc, n.long, n.aliases, n.children)
		return 0, nil

	case *commandNode:
		return a.executeCommand(ctx, n, args, path)
	}
	return 1, fmt.Errorf("unknown node type")
}

func (a *App) executeCommand(ctx context.Context, node *commandNode, args []string, path string) (int, error) {
	handler := node.handler

	// Check for --help before doing anything else
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printCommandHelp(a.output, a.name, path, node.desc, node.long, node.aliases, handler, node.children, a.globals)
			return 0, nil
		}
		if arg == "--" {
			break
		}
	}

	// Build flag set from handler struct tags (also returns pre-scanned fields).
	fs, fields, err := buildFlagSet(handler)
	if err != nil {
		return 1, fmt.Errorf("building flags for %s: %w", path, err)
	}

	// If globals are set, merge their flags into the same flag set.
	var globalFields []fieldInfo
	if a.globals != nil {
		var gf []fieldInfo
		gf, err = addGlobalsToFlagSet(fs, a.globals)
		if err != nil {
			return 1, fmt.Errorf("building global flags: %w", err)
		}
		globalFields = gf
	}

	// Set defaults (handler fields + global fields)
	if err := applyDefaults(fs, fields); err != nil {
		return 1, fmt.Errorf("applying defaults for %s: %w", path, err)
	}
	if err := applyDefaults(fs, globalFields); err != nil {
		return 1, fmt.Errorf("applying global defaults: %w", err)
	}

	// Parse flags
	posArgs, err := fs.Parse(args)
	if err != nil {
		fmt.Fprintf(a.output, "error: %s\n\n", err)
		printCommandHelp(a.output, a.name, path, node.desc, node.long, node.aliases, handler, node.children, a.globals)
		return 1, nil
	}

	// Carry the FlagSet and globals in context.
	ctx = context.WithValue(ctx, flagSetKey{}, fs)
	if a.globals != nil {
		ctx = context.WithValue(ctx, globalsKey{}, a.globals)
	}

	// Load config file if configured
	if a.configFlag != "" {
		configJSON, err := resolveConfigJSON(handler, a.globals, fs, a.configFlag, globalFields, fields)
		if err != nil {
			return 1, fmt.Errorf("loading config: %w", err)
		}
		if configJSON != "" {
			// Apply config tags on globals
			if a.globals != nil {
				if err := applyConfigTags(a.globals, fs, globalFields, configJSON); err != nil {
					return 1, fmt.Errorf("loading config: %w", err)
				}
			}
			// Apply config:"key" struct tags on handler
			if err := applyConfigTags(handler, fs, fields, configJSON); err != nil {
				return 1, fmt.Errorf("loading config: %w", err)
			}
		}
	}

	// CVR lifecycle on globals: Configure → Validate
	if a.globals != nil {
		if c, ok := a.globals.(Configurer); ok {
			if err := c.Configure(); err != nil {
				return 1, fmt.Errorf("globals configure: %w", err)
			}
		}
		if v, ok := a.globals.(Validator); ok {
			if err := v.Validate(); err != nil {
				return 1, fmt.Errorf("globals validate: %w", err)
			}
		}
		// Defer cleanup if globals implements io.Closer
		if cl, ok := a.globals.(io.Closer); ok {
			defer cl.Close()
		}
	}

	// CVR lifecycle on handler: Configure → Validate
	if c, ok := handler.(Configurer); ok {
		if err := c.Configure(); err != nil {
			return 1, fmt.Errorf("configure: %w", err)
		}
	}
	if v, ok := handler.(Validator); ok {
		if err := v.Validate(); err != nil {
			return 1, fmt.Errorf("validate: %w", err)
		}
	}

	// Validate args
	if node.opts.argsFunc != nil {
		if err := node.opts.argsFunc(posArgs); err != nil {
			return 1, err
		}
	}

	// Build middleware chain
	runFn := func(ctx context.Context) error {
		return handler.Run(ctx, posArgs)
	}

	info := CommandInfo{Name: path, Args: posArgs}
	chain := buildChain(a.middlewares, info, runFn)

	// Errors (including ExitError) propagate to App.Run which handles exit codes.
	if err := chain(ctx); err != nil {
		return 1, err
	}
	return 0, nil
}

func buildChain(middlewares []Middleware, info CommandInfo, final func(context.Context) error) func(context.Context) error {
	if len(middlewares) == 0 {
		return final
	}
	// Build from the inside out
	chain := final
	for i := len(middlewares) - 1; i >= 0; i-- {
		m := middlewares[i]
		next := chain
		chain = func(ctx context.Context) error {
			return m(ctx, info, next)
		}
	}
	return chain
}

// defconCmd returns the default config command name, or "" if no
// default config is registered.
func (a *App) defconCmd() string {
	if len(a.defaultConfig) == 0 {
		return ""
	}
	if a.defaultConfigCmd != "" {
		return a.defaultConfigCmd
	}
	return "defcon"
}

func (a *App) handleDefaultConfig() int {
	if len(a.defaultConfig) == 0 {
		fmt.Fprintf(a.output, "no default config registered\n")
		return 1
	}
	if _, err := a.stdout.Write(a.defaultConfig); err != nil {
		fmt.Fprintf(a.output, "error writing config: %s\n", err)
		return 1
	}
	return 0
}

// FlagSetFromContext returns the *FlagSet stored in ctx during command
// execution, or nil if called outside of a command handler.
func FlagSetFromContext(ctx context.Context) *FlagSet {
	fs, _ := ctx.Value(flagSetKey{}).(*FlagSet)
	return fs
}

// GlobalsFromContext retrieves the globals pointer from context, cast to *T.
// Returns nil if no globals were registered or the type doesn't match.
func GlobalsFromContext[T any](ctx context.Context) *T {
	v := ctx.Value(globalsKey{})
	if v == nil {
		return nil
	}
	if g, ok := v.(*T); ok {
		return g
	}
	return nil
}

// IsSet reports whether a flag was explicitly set on the command line.
// Must be called from within Run to return meaningful results.
func IsSet(ctx context.Context, flagName string) bool {
	if fs := FlagSetFromContext(ctx); fs != nil {
		return fs.IsSet(flagName)
	}
	return false
}

// DumpConfig returns the resolved configuration of handler as a map of
// flag name → current field value. It reflects directly over the handler
// struct, so it captures values set by both CLI flags and config files.
func DumpConfig(handler Runnable) map[string]any {
	rv := reflect.ValueOf(handler)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	fields, err := scanFields(rv.Type())
	if err != nil {
		return nil
	}
	m := make(map[string]any, len(fields))
	for _, fi := range fields {
		if fi.flagLong == "" {
			continue
		}
		fieldVal := fieldByIndex(rv, fi.fieldIndex)
		m[fi.flagLong] = fieldVal.Interface()
	}
	return m
}
