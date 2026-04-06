package cli_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/runreveal/lib/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

type echoCmd struct {
	Message string `cli:"message,m" usage:"message to echo" default:"hello"`
	Count   int    `cli:"count,n"   usage:"number of times" default:"1"`
}

func (e *echoCmd) Run(_ context.Context, _ []string) error { return nil }

type boolCmd struct {
	Verbose bool   `cli:"verbose,v" usage:"be verbose"`
	Debug   bool   `cli:"debug,d"   usage:"debug mode"`
	Name    string `cli:"name"      usage:"name"`
}

func (b *boolCmd) Run(_ context.Context, _ []string) error { return nil }

type noopCmd struct{}

func (n *noopCmd) Run(_ context.Context, _ []string) error { return nil }

type errCmd struct{}

func (e *errCmd) Run(_ context.Context, _ []string) error {
	return errors.New("run failed")
}

type captureArgsCmd struct {
	inner func([]string)
}

func (c *captureArgsCmd) Run(_ context.Context, args []string) error {
	c.inner(args)
	return nil
}

type panicCmd struct{}

func (p *panicCmd) Run(_ context.Context, _ []string) error {
	panic("oh no")
}

type exitCodeCmd struct{ code int }

func (e *exitCodeCmd) Run(_ context.Context, _ []string) error {
	return &cli.ExitError{Code: e.code, Err: errors.New("custom exit")}
}

type validateCmd struct {
	Value string `cli:"value" usage:"a value"`
	valid bool
}

func (v *validateCmd) Validate() error {
	if v.Value == "" {
		return errors.New("value is required")
	}
	v.valid = true
	return nil
}
func (v *validateCmd) Run(_ context.Context, _ []string) error {
	if !v.valid {
		return errors.New("Validate was not called")
	}
	return nil
}

type isSetCmd struct {
	Name       string `cli:"name" usage:"name" default:"default"`
	nameWasSet bool
}

func (i *isSetCmd) Run(ctx context.Context, _ []string) error {
	i.nameWasSet = cli.IsSet(ctx, "name")
	return nil
}

// writeConfigFile writes JSON content to a temp file and returns its path.
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	tmp := t.TempDir() + "/config.json"
	require.NoError(t, os.WriteFile(tmp, []byte(content), 0600))
	return tmp
}

// --- flag parsing tests ---

func TestFlagParsing_LongFlag(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo", "--message", "world", "--count", "3"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "world", cmd.Message)
	assert.Equal(t, 3, cmd.Count)
}

func TestFlagParsing_ShortFlag(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo", "-m", "hi", "-n", "2"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "hi", cmd.Message)
	assert.Equal(t, 2, cmd.Count)
}

func TestFlagParsing_EqualsSyntax(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo", "--message=greet", "--count=5"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "greet", cmd.Message)
	assert.Equal(t, 5, cmd.Count)
}

func TestFlagParsing_ShortEqualsSyntax(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo", "-m=yo"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "yo", cmd.Message)
}

func TestFlagParsing_CombinedBoolShorts(t *testing.T) {
	cmd := &boolCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run", "-vd"})
	assert.Equal(t, 0, code)
	assert.True(t, cmd.Verbose)
	assert.True(t, cmd.Debug)
}

func TestFlagParsing_DoubleDashSeparator(t *testing.T) {
	var capturedArgs []string
	handler := &captureArgsCmd{inner: func(args []string) { capturedArgs = args }}

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--", "pos1", "pos2"})
	assert.Equal(t, 0, code)
	assert.Equal(t, []string{"pos1", "pos2"}, capturedArgs)
}

func TestFlagParsing_UnknownFlag(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo", "--unknown"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "unknown flag")
}

func TestFlagParsing_Defaults(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", cmd.Message)
	assert.Equal(t, 1, cmd.Count)
}

// --- struct tag scanning tests ---

type embeddedGlobals struct {
	Verbose bool   `cli:"verbose,v" usage:"verbose mode"`
	Config  string `cli:"config,c"  usage:"config file"  default:"config.json"`
}

type embeddedCmd struct {
	embeddedGlobals
	Port int `cli:"port,p" usage:"port number" default:"8080"`
}

func (e *embeddedCmd) Run(_ context.Context, _ []string) error { return nil }

func TestStructTags_EmbeddedStruct(t *testing.T) {
	cmd := &embeddedCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("serve", "serve", cmd))

	code := app.Run(context.Background(), []string{"serve", "--verbose", "--port", "9090"})
	assert.Equal(t, 0, code)
	assert.True(t, cmd.Verbose)
	assert.Equal(t, 9090, cmd.Port)
	assert.Equal(t, "config.json", cmd.Config) // default
}

type skipCmd struct {
	Name    string `cli:"name" usage:"name"`
	Ignored string `cli:"-"`
	Also    string // no tag, should be ignored
}

func (s *skipCmd) Run(_ context.Context, _ []string) error { return nil }

func TestStructTags_SkipField(t *testing.T) {
	cmd := &skipCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run", "--ignored", "val"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "unknown flag")
}

type ptrCmd struct {
	Name *string `cli:"name" usage:"name"`
	Port *int    `cli:"port" usage:"port"`
}

func (p *ptrCmd) Run(_ context.Context, _ []string) error { return nil }

func TestStructTags_PointerFields(t *testing.T) {
	cmd := &ptrCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run", "--name", "alice", "--port", "3000"})
	assert.Equal(t, 0, code)
	require.NotNil(t, cmd.Name)
	assert.Equal(t, "alice", *cmd.Name)
	require.NotNil(t, cmd.Port)
	assert.Equal(t, 3000, *cmd.Port)
}

type allTypesCmd struct {
	Str  string        `cli:"str"  default:"s"`
	Bool bool          `cli:"bool"`
	I    int           `cli:"int"  default:"1"`
	I64  int64         `cli:"i64"  default:"2"`
	U    uint          `cli:"uint" default:"3"`
	U64  uint64        `cli:"u64"  default:"4"`
	F    float64       `cli:"flt"  default:"1.5"`
	Dur  time.Duration `cli:"dur"  default:"5s"`
	Strs []string      `cli:"strs"`
}

func (a *allTypesCmd) Run(_ context.Context, _ []string) error { return nil }

func TestStructTags_AllTypes(t *testing.T) {
	cmd := &allTypesCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{
		"run",
		"--str", "hello",
		"--bool",
		"--int", "10",
		"--i64", "20",
		"--uint", "30",
		"--u64", "40",
		"--flt", "3.14",
		"--dur", "10s",
		"--strs", "a",
		"--strs", "b",
	})
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", cmd.Str)
	assert.True(t, cmd.Bool)
	assert.Equal(t, 10, cmd.I)
	assert.Equal(t, int64(20), cmd.I64)
	assert.Equal(t, uint(30), cmd.U)
	assert.Equal(t, uint64(40), cmd.U64)
	assert.InDelta(t, 3.14, cmd.F, 0.001)
	assert.Equal(t, 10*time.Second, cmd.Dur)
	assert.Equal(t, []string{"a", "b"}, cmd.Strs)
}

// --- command routing tests ---

func TestRouting_Subcommand(t *testing.T) {
	var called string
	sub := &captureArgsCmd{inner: func(_ []string) { called = "sub" }}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("parent", "parent", &noopCmd{},
		cli.Command("child", "child", sub),
	))

	code := app.Run(context.Background(), []string{"parent", "child"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "sub", called)
}

func TestRouting_GroupPrintsHelp(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Group("admin", "admin commands",
		cli.Command("migrate", "run migrations", &noopCmd{}),
	))

	code := app.Run(context.Background(), []string{"admin"})
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "migrate")
}

func TestRouting_UnknownCommand(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("serve", "serve", &noopCmd{}))

	code := app.Run(context.Background(), []string{"unknown"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "unknown command")
}

func TestRouting_NestedGroup(t *testing.T) {
	var called bool
	sub := &captureArgsCmd{inner: func(_ []string) { called = true }}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Group("a", "a",
		cli.Group("b", "b",
			cli.Command("c", "c", sub),
		),
	))

	code := app.Run(context.Background(), []string{"a", "b", "c"})
	assert.Equal(t, 0, code)
	assert.True(t, called)
}

// --- lifecycle tests ---

func TestLifecycle_ValidateCalled(t *testing.T) {
	cmd := &validateCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run", "--value", "set"})
	assert.Equal(t, 0, code)
}

func TestLifecycle_ValidateError(t *testing.T) {
	cmd := &validateCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "value is required")
}

func TestLifecycle_MiddlewareOrdering(t *testing.T) {
	var order []string
	mkMiddleware := func(name string) cli.Middleware {
		return func(ctx context.Context, info cli.CommandInfo, next func(context.Context) error) error {
			order = append(order, name+":before")
			err := next(ctx)
			order = append(order, name+":after")
			return err
		}
	}

	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithMiddleware(mkMiddleware("first")),
		cli.WithMiddleware(mkMiddleware("second")),
	)
	app.AddCommand(cli.Command("run", "run", &noopCmd{}))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 0, code)
	assert.Equal(t, []string{"first:before", "second:before", "second:after", "first:after"}, order)
}

func TestLifecycle_RunError(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", &errCmd{}))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "run failed")
}

func TestLifecycle_PanicRecovery(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", &panicCmd{}))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 1, code)
}

func TestLifecycle_ExitError(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", &exitCodeCmd{code: 42}))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 42, code)
}

// --- help output tests ---

func TestHelp_AppHelp(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "My application", cli.WithOutput(&buf), cli.WithVersion("1.0"))
	app.AddCommand(cli.Command("serve", "Start HTTP server", &noopCmd{}))

	code := app.Run(context.Background(), []string{"--help"})
	assert.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "myapp")
	assert.Contains(t, out, "serve")
	assert.Contains(t, out, "--help")
}

func TestHelp_CommandHelp(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo something", cmd))

	code := app.Run(context.Background(), []string{"echo", "--help"})
	assert.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "--message")
	assert.Contains(t, out, "--count")
	assert.Contains(t, out, "default: hello")
}

func TestHelp_VersionFlag(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf), cli.WithVersion("2.3.4"))

	code := app.Run(context.Background(), []string{"--version"})
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "2.3.4")
}

func TestHelp_ShortAlias(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	app.Run(context.Background(), []string{"echo", "--help"})
	out := buf.String()
	assert.True(t, strings.Contains(out, "-m") || strings.Contains(out, "--message"))
}

// --- edge cases ---

func TestEdge_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("serve", "serve", &noopCmd{}))

	code := app.Run(context.Background(), []string{})
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "serve")
}

func TestEdge_ArgsValidation_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", &noopCmd{}, cli.WithArgs(cli.NoArgs)))

	code := app.Run(context.Background(), []string{"run", "extra"})
	assert.Equal(t, 1, code)
}

func TestEdge_ArgsValidation_ExactArgs(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", &noopCmd{}, cli.WithArgs(cli.ExactArgs(2))))

	code := app.Run(context.Background(), []string{"run", "a", "b"})
	assert.Equal(t, 0, code)

	code = app.Run(context.Background(), []string{"run", "a"})
	assert.Equal(t, 1, code)
}

func TestEdge_IsSet(t *testing.T) {
	setCmd := &isSetCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", setCmd))

	app.Run(context.Background(), []string{"run", "--name", "alice"})
	assert.True(t, setCmd.nameWasSet)
}

func TestEdge_IsSet_NotSet(t *testing.T) {
	setCmd := &isSetCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", setCmd))

	app.Run(context.Background(), []string{"run"})
	assert.False(t, setCmd.nameWasSet)
}

// --- config file loading tests ---

type dbSection struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type configHandler struct {
	ConfigFile string    `cli:"config,c" usage:"config file"`
	DB         dbSection `                                   config:"database"`
}

func (c *configHandler) Run(_ context.Context, _ []string) error { return nil }

func TestConfig_BasicLoad(t *testing.T) {
	handler := &configHandler{}
	f := writeConfigFile(t, `{
		"database": {
			"host": "localhost",
			"port": 5432
		}
	}`)

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "localhost", handler.DB.Host)
	assert.Equal(t, 5432, handler.DB.Port)
}

func TestConfig_MissingFileSilent(t *testing.T) {
	handler := &configHandler{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 0, code)
}

type overrideableHandler struct {
	ConfigFile string `cli:"config" usage:"config file"`
	Host       string `cli:"host"   usage:"host"        default:"flag-default" config:"host"`
}

func (o *overrideableHandler) Run(_ context.Context, _ []string) error { return nil }

func TestConfig_CLIFlagOverridesConfig(t *testing.T) {
	handler := &overrideableHandler{}
	f := writeConfigFile(t, `{"host": "from-config"}`)

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--config", f, "--host", "from-flag"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "from-flag", handler.Host)
}

func TestConfig_ConfigLoadsWhenNotExplicit(t *testing.T) {
	handler := &overrideableHandler{}
	f := writeConfigFile(t, `{"host": "from-config"}`)

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	// Config file set but --host not set, so config value should apply
	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "from-config", handler.Host)
}

type nestedVal struct {
	Val string `json:"val"`
}
type nestedConfigHandler struct {
	ConfigFile string    `cli:"config" usage:"config file"`
	Nested     nestedVal `                                 config:"a.b"`
}

func (n *nestedConfigHandler) Run(_ context.Context, _ []string) error { return nil }

func TestConfig_NestedPath(t *testing.T) {
	handler := &nestedConfigHandler{}
	f := writeConfigFile(t, `{
		"a": {
			"b": {
				"val": "deep"
			}
		}
	}`)

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "deep", handler.Nested.Val)
}

type rootSection struct {
	Foo string `json:"foo"`
	Bar int    `json:"bar"`
}
type rootConfigHandler struct {
	ConfigFile string      `cli:"config" usage:"config file"`
	All        rootSection `                                 config:"."`
}

func (r *rootConfigHandler) Run(_ context.Context, _ []string) error { return nil }

func TestConfig_RootPath(t *testing.T) {
	handler := &rootConfigHandler{}
	f := writeConfigFile(t, `{"foo": "baz", "bar": 99}`)

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "baz", handler.All.Foo)
	assert.Equal(t, 99, handler.All.Bar)
}

func TestMiddleware_CommandInfo(t *testing.T) {
	var capturedInfo cli.CommandInfo
	mw := func(ctx context.Context, info cli.CommandInfo, next func(context.Context) error) error {
		capturedInfo = info
		return next(ctx)
	}

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithMiddleware(mw))
	app.AddCommand(cli.Group("admin", "admin",
		cli.Command("migrate", "migrate", &noopCmd{}),
	))

	code := app.Run(context.Background(), []string{"admin", "migrate"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "admin migrate", capturedInfo.Name)
}

func TestConfig_HuJSON(t *testing.T) {
	handler := &configHandler{}
	// HuJSON allows C-style comments and trailing commas.
	f := writeConfigFile(t, `{
		// database connection settings
		"database": {
			"host": "hujson-host",
			"port": 5433, // trailing comma
		}
	}`)

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "hujson-host", handler.DB.Host)
	assert.Equal(t, 5433, handler.DB.Port)
}

// --- WithGlobals tests ---

type testGlobals struct {
	Verbose bool   `cli:"verbose,v" usage:"verbose"`
	Config  string `cli:"config,c"  usage:"config file" default:"config.json"`
}

type simpleServeCmd struct {
	Addr    string `cli:"addr" usage:"listen address" default:":8080"`
	globals *testGlobals
}

func (s *simpleServeCmd) Run(ctx context.Context, _ []string) error {
	s.globals = cli.GlobalsFromContext[testGlobals](ctx)
	return nil
}

func TestGlobals_FlagsAvailable(t *testing.T) {
	g := &testGlobals{}
	cmd := &simpleServeCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithGlobals(g))
	app.AddCommand(cli.Command("serve", "serve", cmd))

	code := app.Run(context.Background(), []string{"serve", "--verbose", "--addr", ":9090"})
	assert.Equal(t, 0, code)
	assert.True(t, g.Verbose)
	assert.Equal(t, ":9090", cmd.Addr)
}

func TestGlobals_FromContext(t *testing.T) {
	g := &testGlobals{}
	cmd := &simpleServeCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithGlobals(g))
	app.AddCommand(cli.Command("serve", "serve", cmd))

	code := app.Run(context.Background(), []string{"serve", "--verbose"})
	assert.Equal(t, 0, code)
	require.NotNil(t, cmd.globals)
	assert.True(t, cmd.globals.Verbose)
	assert.Equal(t, "config.json", cmd.globals.Config) // default
}

func TestGlobals_ConfigFlagOnGlobals(t *testing.T) {
	type serveWithConfig struct {
		DB dbSection `config:"database"`
	}
	handler := &struct {
		serveWithConfig
	}{}
	handler.serveWithConfig = serveWithConfig{}

	// Use a handler that has config tags but no config flag —
	// the config flag is on globals.
	g := &testGlobals{}
	f := writeConfigFile(t, `{"database": {"host": "global-host", "port": 3306}}`)

	configCmd := &configHandler{DB: dbSection{}}
	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithGlobals(g),
		cli.WithConfigFlag("config"),
	)
	app.AddCommand(cli.Command("run", "run", configCmd))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "global-host", configCmd.DB.Host)
	assert.Equal(t, 3306, configCmd.DB.Port)
}

// --- ConfigAt tests ---

type configAtCmd struct {
	Addr string `cli:"addr" usage:"listen address" default:":8080"`
	DB   dbSection
}

func (c *configAtCmd) Run(_ context.Context, _ []string) error { return nil }

func TestConfigAt_BasicLoad(t *testing.T) {
	g := &testGlobals{}
	cmd := &configAtCmd{}
	f := writeConfigFile(t, `{
		"database": {"host": "configat-host", "port": 5432}
	}`)

	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithGlobals(g),
		cli.WithConfigFlag("config"),
	)
	app.AddCommand(cli.Command("serve", "serve", cmd,
		cli.ConfigAt("database", &cmd.DB),
	))

	code := app.Run(context.Background(), []string{"serve", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "configat-host", cmd.DB.Host)
	assert.Equal(t, 5432, cmd.DB.Port)
}

func TestConfigAt_NestedKey(t *testing.T) {
	g := &testGlobals{}
	cmd := &configAtCmd{}
	f := writeConfigFile(t, `{
		"services": {
			"api": {"host": "nested-host", "port": 9090}
		}
	}`)

	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithGlobals(g),
		cli.WithConfigFlag("config"),
	)
	app.AddCommand(cli.Command("serve", "serve", cmd,
		cli.ConfigAt("services.api", &cmd.DB),
	))

	code := app.Run(context.Background(), []string{"serve", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "nested-host", cmd.DB.Host)
	assert.Equal(t, 9090, cmd.DB.Port)
}

func TestConfigAt_MissingSectionSilent(t *testing.T) {
	g := &testGlobals{}
	cmd := &configAtCmd{}
	f := writeConfigFile(t, `{"other": {}}`)

	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithGlobals(g),
		cli.WithConfigFlag("config"),
	)
	app.AddCommand(cli.Command("serve", "serve", cmd,
		cli.ConfigAt("database", &cmd.DB),
	))

	code := app.Run(context.Background(), []string{"serve", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "", cmd.DB.Host) // not populated
}

func TestConfig_EnvVarReplacement(t *testing.T) {
	handler := &overrideableHandler{}
	t.Setenv("CLI_TEST_HOST", "env-replaced-host")
	f := writeConfigFile(t, `{"host": "$CLI_TEST_HOST"}`)

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "env-replaced-host", handler.Host)
}
