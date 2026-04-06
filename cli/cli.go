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
	"strings"
)

// flagSetKey is the context key used to carry the *FlagSet during command execution.
type flagSetKey struct{}

// globalsKey is the context key used to carry the globals pointer during execution.
type globalsKey struct{}

// configBinding pairs a config file key with a destination pointer for ConfigAt.
type configBinding struct {
	key string
	dst any
}

// Runnable is the core interface every command handler must implement.
type Runnable interface {
	Run(ctx context.Context, args []string) error
}

// Validator is optionally implemented by handlers to validate config after loading.
type Validator interface {
	Validate() error
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
	children []Node
}

func (g *groupNode) nodeName() string { return g.name }
func (g *groupNode) nodeDesc() string { return g.desc }
func (g *groupNode) isGroup() bool    { return true }

// CmdOption configures a Command node.
type CmdOption func(*cmdOptions)

type cmdOptions struct {
	argsFunc       ArgsFunc
	configBindings []configBinding
}

// WithArgs sets an args validation function on a command.
func WithArgs(f ArgsFunc) CmdOption {
	return func(o *cmdOptions) { o.argsFunc = f }
}

// ConfigAt registers a config file section to be unmarshaled into dst.
// key is a dot-separated path into the config file JSON (e.g. "serve", "common.db").
// Use "." for the entire config root. dst must be a pointer.
func ConfigAt(key string, dst any) CmdOption {
	return func(o *cmdOptions) {
		o.configBindings = append(o.configBindings, configBinding{key: key, dst: dst})
	}
}

// Command creates a command node. Each element of opts may be a Node (child
// subcommand) or a CmdOption (behavioural option); they are distinguished by
// type at runtime.
func Command(name, desc string, handler Runnable, opts ...any) Node {
	o := cmdOptions{}
	var children []Node
	for _, opt := range opts {
		switch v := opt.(type) {
		case Node:
			children = append(children, v)
		case CmdOption:
			v(&o)
		}
	}
	return &commandNode{name: name, desc: desc, handler: handler, children: children, opts: o}
}

// Group creates a group node that only prints help when invoked directly.
func Group(name, desc string, children ...Node) Node {
	return &groupNode{name: name, desc: desc, children: children}
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
func WithGlobals(ptr any) AppOption {
	return func(a *App) { a.globals = ptr }
}

// WithOutput sets the writer for help/error output (default: os.Stderr).
func WithOutput(w io.Writer) AppOption {
	return func(a *App) { a.output = w }
}

// App is the top-level CLI application.
type App struct {
	name        string
	desc        string
	version     string
	configFlag  string
	globals     any // pointer to globals struct, if set
	middlewares []Middleware
	children    []Node
	output      io.Writer
}

// New creates a new App.
func New(name, desc string, opts ...AppOption) *App {
	a := &App{
		name:   name,
		desc:   desc,
		output: os.Stderr,
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
		printAppHelp(a.output, a.name, a.desc, a.children, a.version)
		return 0, nil
	}

	node, rest, path := routeArgsWithPath(a.children, args, "")
	if node == nil {
		// Unknown command
		fmt.Fprintf(a.output, "unknown command %q\n\n", args[0])
		printAppHelp(a.output, a.name, a.desc, a.children, a.version)
		return 1, nil
	}

	return a.executeNode(ctx, node, rest, path)
}

func routeArgsWithPath(children []Node, args []string, prefix string) (Node, []string, string) {
	if len(args) == 0 {
		return nil, args, prefix
	}

	name := args[0]
	// Don't treat flags as command names
	if strings.HasPrefix(name, "-") {
		return nil, args, prefix
	}

	for _, child := range children {
		if child.nodeName() == name {
			fullPath := name
			if prefix != "" {
				fullPath = prefix + " " + name
			}
			rest := args[1:]

			// If this node has children and the next arg matches one, recurse
			var subChildren []Node
			switch n := child.(type) {
			case *commandNode:
				subChildren = n.children
			case *groupNode:
				subChildren = n.children
			}

			if len(subChildren) > 0 && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
				if sub, subRest, subPath := routeArgsWithPath(subChildren, rest, fullPath); sub != nil {
					return sub, subRest, subPath
				}
			}

			return child, rest, fullPath
		}
	}
	return nil, args, prefix
}

func (a *App) executeNode(ctx context.Context, node Node, args []string, path string) (int, error) {
	switch n := node.(type) {
	case *groupNode:
		// Groups print help when invoked directly (no matching subcommand)
		printGroupHelp(a.output, a.name, path, n.desc, n.children)
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
			printCommandHelp(a.output, a.name, path, node.desc, handler, node.children, a.globals)
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
		printCommandHelp(a.output, a.name, path, node.desc, handler, node.children, a.globals)
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
			// Apply config:"key" struct tags on handler
			if err := applyConfigTags(handler, fs, fields, configJSON); err != nil {
				return 1, fmt.Errorf("loading config: %w", err)
			}
			// Apply ConfigAt bindings
			if err := applyConfigBindings(node.opts.configBindings, configJSON); err != nil {
				return 1, fmt.Errorf("loading config: %w", err)
			}
		}
	}

	// Validate
	if v, ok := handler.(Validator); ok {
		if err := v.Validate(); err != nil {
			return 1, err
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

	if err := chain(ctx); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code, exitErr
		}
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
