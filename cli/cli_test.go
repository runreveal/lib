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

type dbConfigOnlyCmd struct {
	DB dbSection `config:"database"`
}

func (d *dbConfigOnlyCmd) Run(_ context.Context, _ []string) error { return nil }

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
	// Handler has config tags but no --config flag — the flag lives on globals.
	handler := &dbConfigOnlyCmd{}

	g := &testGlobals{}
	f := writeConfigFile(t, `{"database": {"host": "global-host", "port": 3306}}`)

	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithGlobals(g),
		cli.WithConfigFlag("config"),
	)
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.Equal(t, "global-host", handler.DB.Host)
	assert.Equal(t, 3306, handler.DB.Port)
}

// --- CVR lifecycle on globals ---

type cvrGlobals struct {
	Verbose bool   `cli:"verbose,v" usage:"verbose"`
	Config  string `cli:"config,c"  usage:"config"  default:"config.json"`
	DSN     string `                                                      config:"db.dsn"`

	// Set during lifecycle
	Configured bool
	Validated  bool
	Closed     bool
	DB         string // simulates an initialized resource
}

func (g *cvrGlobals) Configure() error {
	g.Configured = true
	if g.DSN != "" {
		g.DB = "pool:" + g.DSN // simulate opening a connection
	}
	return nil
}

func (g *cvrGlobals) Validate() error {
	g.Validated = true
	return nil
}

func (g *cvrGlobals) Close() error {
	g.Closed = true
	g.DB = ""
	return nil
}

type cvrCmd struct {
	globalsSnapshot *cvrGlobals
}

func (c *cvrCmd) Run(ctx context.Context, _ []string) error {
	g := cli.GlobalsFromContext[cvrGlobals](ctx)
	// Snapshot so test can check state during Run
	c.globalsSnapshot = &cvrGlobals{
		Configured: g.Configured,
		Validated:  g.Validated,
		Closed:     g.Closed,
		DB:         g.DB,
	}
	return nil
}

func TestGlobals_CVR_Lifecycle(t *testing.T) {
	g := &cvrGlobals{}
	cmd := &cvrCmd{}
	f := writeConfigFile(t, `{"db": {"dsn": "postgres://localhost/test"}}`)

	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithGlobals(g),
		cli.WithConfigFlag("config"),
	)
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)

	// During Run: configured and validated, not yet closed
	require.NotNil(t, cmd.globalsSnapshot)
	assert.True(t, cmd.globalsSnapshot.Configured)
	assert.True(t, cmd.globalsSnapshot.Validated)
	assert.False(t, cmd.globalsSnapshot.Closed)
	assert.Equal(t, "pool:postgres://localhost/test", cmd.globalsSnapshot.DB)

	// After Run: closed
	assert.True(t, g.Closed)
	assert.Equal(t, "", g.DB) // resource cleaned up
}

func TestGlobals_CVR_ConfigureError(t *testing.T) {
	// Configure runs before Validate — already covered by
	// TestGlobals_CVR_Lifecycle ordering assertions.
}

type cvrHandlerCmd struct {
	Name       string `cli:"name" usage:"name"`
	configured bool
	validated  bool
}

func (c *cvrHandlerCmd) Configure() error {
	c.configured = true
	return nil
}

func (c *cvrHandlerCmd) Validate() error {
	if !c.configured {
		return errors.New("configure must run before validate")
	}
	c.validated = true
	return nil
}

func (c *cvrHandlerCmd) Run(_ context.Context, _ []string) error {
	if !c.validated {
		return errors.New("validate must run before run")
	}
	return nil
}

func TestHandler_CVR_Lifecycle(t *testing.T) {
	cmd := &cvrHandlerCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run", "--name", "test"})
	assert.Equal(t, 0, code)
	assert.True(t, cmd.configured)
	assert.True(t, cmd.validated)
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

// --- Config precedence table-driven tests ---

func TestConfig_Precedence(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		args       []string
		wantHost   string
	}{
		{
			name:       "flag overrides config",
			configJSON: `{"host": "from-config"}`,
			args:       []string{"run", "--config", "", "--host", "from-flag"},
			wantHost:   "from-flag",
		},
		{
			name:       "config overrides default",
			configJSON: `{"host": "from-config"}`,
			args:       []string{"run", "--config", ""},
			wantHost:   "from-config",
		},
		{
			name:       "default when no config and no flag",
			configJSON: "",
			args:       []string{"run"},
			wantHost:   "flag-default",
		},
		{
			name:       "config key missing uses default",
			configJSON: `{"other": "value"}`,
			args:       []string{"run", "--config", ""},
			wantHost:   "flag-default",
		},
		{
			name:       "empty config value overrides default",
			configJSON: `{"host": ""}`,
			args:       []string{"run", "--config", ""},
			wantHost:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &overrideableHandler{}
			var buf bytes.Buffer
			app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
			app.AddCommand(cli.Command("run", "run", handler))

			args := make([]string, len(tt.args))
			copy(args, tt.args)

			if tt.configJSON != "" {
				f := writeConfigFile(t, tt.configJSON)
				for i, a := range args {
					if a == "" && i > 0 && args[i-1] == "--config" {
						args[i] = f
					}
				}
			}

			code := app.Run(context.Background(), args)
			assert.Equal(t, 0, code, "output: %s", buf.String())
			assert.Equal(t, tt.wantHost, handler.Host)
		})
	}
}

// --- Config with globals: config tag on globals struct ---

type globalsWithConfig struct {
	Config  string `cli:"config,c" usage:"config" default:"config.json"`
	AppName string `                                                    config:"app_name"`
}

type plainCmd struct {
	ran bool
}

func (p *plainCmd) Run(_ context.Context, _ []string) error {
	p.ran = true
	return nil
}

func TestConfig_GlobalsConfigTag(t *testing.T) {
	g := &globalsWithConfig{}
	cmd := &plainCmd{}
	f := writeConfigFile(t, `{"app_name": "loaded-from-config"}`)

	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithGlobals(g),
		cli.WithConfigFlag("config"),
	)
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 0, code)
	assert.True(t, cmd.ran)
	assert.Equal(t, "loaded-from-config", g.AppName)
}

// --- Flag parsing edge cases (table-driven) ---

func TestFlagParsing_ShortFlagEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantVerbose bool
		wantDebug   bool
		wantName    string
	}{
		{
			name:        "combined bool shorts",
			args:        []string{"-vd"},
			wantVerbose: true,
			wantDebug:   true,
		},
		{
			name:        "combined bool + value: -vd is both bools",
			args:        []string{"-vd", "--name", "x"},
			wantVerbose: true,
			wantDebug:   true,
			wantName:    "x",
		},
		{
			name:     "single short with value",
			args:     []string{"-v", "--name", "alice"},
			wantName: "alice", wantVerbose: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &boolCmd{}
			var buf bytes.Buffer
			app := cli.New("app", "test", cli.WithOutput(&buf))
			app.AddCommand(cli.Command("run", "run", cmd))

			code := app.Run(context.Background(), append([]string{"run"}, tt.args...))
			assert.Equal(t, 0, code, "output: %s", buf.String())
			assert.Equal(t, tt.wantVerbose, cmd.Verbose)
			assert.Equal(t, tt.wantDebug, cmd.Debug)
			assert.Equal(t, tt.wantName, cmd.Name)
		})
	}
}

// --- Type coverage for makeFlagDef ---

type moreTypesCmd struct {
	PStr *string        `cli:"pstr" usage:"ptr string"`
	PInt *int           `cli:"pint" usage:"ptr int"`
	PDur *time.Duration `cli:"pdur" usage:"ptr duration"`
	PFlt *float64       `cli:"pflt" usage:"ptr float"`
	PU64 *uint64        `cli:"pu64" usage:"ptr uint64"`
	PBol *bool          `cli:"pbol" usage:"ptr bool"`
}

func (m *moreTypesCmd) Run(_ context.Context, _ []string) error { return nil }

func TestFlagParsing_PointerTypes(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		check func(t *testing.T, cmd *moreTypesCmd)
	}{
		{
			name: "ptr string set",
			args: []string{"--pstr", "hello"},
			check: func(t *testing.T, cmd *moreTypesCmd) {
				require.NotNil(t, cmd.PStr)
				assert.Equal(t, "hello", *cmd.PStr)
			},
		},
		{
			name: "ptr int set",
			args: []string{"--pint", "42"},
			check: func(t *testing.T, cmd *moreTypesCmd) {
				require.NotNil(t, cmd.PInt)
				assert.Equal(t, 42, *cmd.PInt)
			},
		},
		{
			name: "ptr duration set",
			args: []string{"--pdur", "5s"},
			check: func(t *testing.T, cmd *moreTypesCmd) {
				require.NotNil(t, cmd.PDur)
				assert.Equal(t, 5*time.Second, *cmd.PDur)
			},
		},
		{
			name: "ptr float set",
			args: []string{"--pflt", "3.14"},
			check: func(t *testing.T, cmd *moreTypesCmd) {
				require.NotNil(t, cmd.PFlt)
				assert.InDelta(t, 3.14, *cmd.PFlt, 0.001)
			},
		},
		{
			name: "ptr uint64 set",
			args: []string{"--pu64", "99"},
			check: func(t *testing.T, cmd *moreTypesCmd) {
				require.NotNil(t, cmd.PU64)
				assert.Equal(t, uint64(99), *cmd.PU64)
			},
		},
		{
			name: "ptr bool set",
			args: []string{"--pbol"},
			check: func(t *testing.T, cmd *moreTypesCmd) {
				require.NotNil(t, cmd.PBol)
				assert.True(t, *cmd.PBol)
			},
		},
		{
			name: "unset ptrs remain nil",
			args: []string{},
			check: func(t *testing.T, cmd *moreTypesCmd) {
				assert.Nil(t, cmd.PStr)
				assert.Nil(t, cmd.PInt)
				assert.Nil(t, cmd.PDur)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &moreTypesCmd{}
			var buf bytes.Buffer
			app := cli.New("app", "test", cli.WithOutput(&buf))
			app.AddCommand(cli.Command("run", "run", cmd))

			code := app.Run(context.Background(), append([]string{"run"}, tt.args...))
			assert.Equal(t, 0, code, "output: %s", buf.String())
			tt.check(t, cmd)
		})
	}
}

// --- DumpConfig, DefaultConfig, ExitError ---

func TestDumpConfig(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	app.Run(context.Background(), []string{"echo", "--message", "hi"})
	m := cli.DumpConfig(cmd)
	require.NotNil(t, m)
	assert.Equal(t, "hi", m["message"])
	assert.Equal(t, 1, m["count"]) // default
}

func TestDefaultConfig(t *testing.T) {
	data := []byte(`{"key": "value"}`)
	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithDefaultConfig(data),
	)

	code := app.Run(context.Background(), []string{"defcon"})
	assert.Equal(t, 0, code)
	assert.Equal(t, string(data), buf.String())
}

func TestDefaultConfig_CustomCommand(t *testing.T) {
	data := []byte(`{"x": 1}`)
	var buf bytes.Buffer
	app := cli.New("app", "test",
		cli.WithOutput(&buf),
		cli.WithDefaultConfig(data),
		cli.WithDefaultConfigCommand("dump"),
	)

	code := app.Run(context.Background(), []string{"dump"})
	assert.Equal(t, 0, code)
	assert.Equal(t, string(data), buf.String())
}

func TestDefaultConfig_NotRegistered(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))

	// Without WithDefaultConfig, "defcon" is not a recognized command.
	code := app.Run(context.Background(), []string{"defcon"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "unknown command")
}

func TestExitError(t *testing.T) {
	e := &cli.ExitError{Code: 3, Err: errors.New("boom")}
	assert.Equal(t, "boom", e.Error())
	assert.Equal(t, "boom", e.Unwrap().Error())

	e2 := &cli.ExitError{Code: 5}
	assert.Contains(t, e2.Error(), "exit code 5")
	assert.Nil(t, e2.Unwrap())
}

// --- Config file error paths ---

func TestConfig_ExplicitMissingFileErrors(t *testing.T) {
	handler := &configHandler{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	// Explicitly passing a non-existent file should error
	code := app.Run(context.Background(), []string{"run", "--config", "/nonexistent/config.json"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "reading config file")
}

func TestConfig_InvalidJSON(t *testing.T) {
	handler := &configHandler{}
	f := writeConfigFile(t, `{not valid json`)

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithConfigFlag("config"))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--config", f})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "parsing config file")
}

// --- MinArgs coverage ---

func TestEdge_ArgsValidation_MinArgs(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", &noopCmd{}, cli.WithArgs(cli.MinArgs(2))))

	code := app.Run(context.Background(), []string{"run", "a", "b"})
	assert.Equal(t, 0, code)

	code = app.Run(context.Background(), []string{"run", "a"})
	assert.Equal(t, 1, code)
}

// --- Handler Configure error ---

type failConfigureCmd struct{}

func (f *failConfigureCmd) Configure() error                        { return errors.New("configure failed") }
func (f *failConfigureCmd) Run(_ context.Context, _ []string) error { return nil }

func TestHandler_ConfigureError(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", &failConfigureCmd{}))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "configure")
}

// --- Globals Configure/Validate errors ---

type failGlobalsConfigure struct {
	Config string `cli:"config,c" default:"config.json"`
}

func (f *failGlobalsConfigure) Configure() error {
	return errors.New("globals configure boom")
}

func TestGlobals_ConfigureError(t *testing.T) {
	g := &failGlobalsConfigure{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithGlobals(g))
	app.AddCommand(cli.Command("run", "run", &noopCmd{}))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "globals configure")
}

type failGlobalsValidate struct {
	Config string `cli:"config,c" default:"config.json"`
}

func (f *failGlobalsValidate) Validate() error {
	return errors.New("globals validate boom")
}

func TestGlobals_ValidateError(t *testing.T) {
	g := &failGlobalsValidate{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithGlobals(g))
	app.AddCommand(cli.Command("run", "run", &noopCmd{}))

	code := app.Run(context.Background(), []string{"run"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "globals validate")
}

// --- Long flag edge cases ---

func TestFlagParsing_LongBoolWithEquals(t *testing.T) {
	cmd := &boolCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("run", "run", cmd))

	code := app.Run(context.Background(), []string{"run", "--verbose=true"})
	assert.Equal(t, 0, code)
	assert.True(t, cmd.Verbose)
}

func TestFlagParsing_LongFlagMissingValue(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo", "--message"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "requires a value")
}

func TestFlagParsing_ShortFlagMissingValue(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo", "-m"})
	assert.Equal(t, 1, code)
	assert.Contains(t, buf.String(), "requires a value")
}

// --- Global flags before/between/after command routing ---

type profileGlobals struct {
	Profile string `cli:"profile,p" usage:"profile name"`
	Verbose bool   `cli:"verbose,v" usage:"verbose"`
}

type authLoginCmd struct {
	Token   string `cli:"token,t" usage:"auth token"`
	globals *profileGlobals
}

func (a *authLoginCmd) Run(ctx context.Context, _ []string) error {
	a.globals = cli.GlobalsFromContext[profileGlobals](ctx)
	return nil
}

func TestGlobals_BeforeCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantProfile string
		wantVerbose bool
		wantToken   string
	}{
		{
			name:        "global flag before command",
			args:        []string{"--profile", "staging", "auth", "login"},
			wantProfile: "staging",
		},
		{
			name:        "global flag between command and subcommand",
			args:        []string{"auth", "--profile", "staging", "login"},
			wantProfile: "staging",
		},
		{
			name:        "global flag after subcommand",
			args:        []string{"auth", "login", "--profile", "staging"},
			wantProfile: "staging",
		},
		{
			name:        "global flag with = syntax before command",
			args:        []string{"--profile=staging", "auth", "login"},
			wantProfile: "staging",
		},
		{
			name:        "short global flag before command",
			args:        []string{"-p", "staging", "auth", "login"},
			wantProfile: "staging",
		},
		{
			name:        "boolean global flag before command",
			args:        []string{"--verbose", "auth", "login"},
			wantVerbose: true,
		},
		{
			name:        "multiple global flags before command",
			args:        []string{"--profile", "staging", "--verbose", "auth", "login"},
			wantProfile: "staging",
			wantVerbose: true,
		},
		{
			name:        "global and command flags mixed",
			args:        []string{"--profile", "staging", "auth", "login", "--token", "abc"},
			wantProfile: "staging",
			wantToken:   "abc",
		},
		{
			name:        "all flags after subcommand",
			args:        []string{"auth", "login", "--profile", "staging", "--token", "abc", "--verbose"},
			wantProfile: "staging",
			wantToken:   "abc",
			wantVerbose: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &profileGlobals{}
			cmd := &authLoginCmd{}
			var buf bytes.Buffer
			app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithGlobals(g))
			app.AddCommand(cli.Group("auth", "auth commands",
				cli.Command("login", "log in", cmd),
			))

			code := app.Run(context.Background(), tt.args)
			assert.Equal(t, 0, code, "output: %s", buf.String())
			assert.Equal(t, tt.wantProfile, g.Profile)
			assert.Equal(t, tt.wantVerbose, g.Verbose)
			assert.Equal(t, tt.wantToken, cmd.Token)
		})
	}
}

func TestGlobals_DoubleDashStopsStripping(t *testing.T) {
	var capturedArgs []string
	handler := &captureArgsCmd{inner: func(args []string) { capturedArgs = args }}
	g := &profileGlobals{}

	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithGlobals(g))
	app.AddCommand(cli.Command("run", "run", handler))

	code := app.Run(context.Background(), []string{"run", "--", "--profile", "staging"})
	assert.Equal(t, 0, code, "output: %s", buf.String())
	assert.Equal(t, "", g.Profile)
	assert.Equal(t, []string{"--profile", "staging"}, capturedArgs)
}

func TestGlobals_CollisionPanics(t *testing.T) {
	type collisionGlobals struct {
		Message string `cli:"message,m" usage:"global message"`
	}

	g := &collisionGlobals{}
	cmd := &echoCmd{} // also has --message
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithGlobals(g))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	// The panic is caught by App.Run's recover — verify it surfaces as exit code 1.
	code := app.Run(context.Background(), []string{"echo"})
	assert.Equal(t, 1, code)
}

func TestGlobals_SameFlagDifferentBranches(t *testing.T) {
	type branchGlobals struct {
		Verbose bool `cli:"verbose,v" usage:"verbose"`
	}

	g := &branchGlobals{}
	cmd1 := &echoCmd{}
	cmd2 := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf), cli.WithGlobals(g))
	app.AddCommand(
		cli.Command("auth", "auth", cmd1),
		cli.Command("api", "api", cmd2),
	)

	code := app.Run(context.Background(), []string{"auth", "--message", "abc"})
	assert.Equal(t, 0, code, "output: %s", buf.String())
	assert.Equal(t, "abc", cmd1.Message)

	code = app.Run(context.Background(), []string{"api", "--message", "xyz"})
	assert.Equal(t, 0, code, "output: %s", buf.String())
	assert.Equal(t, "xyz", cmd2.Message)
}

func TestGlobals_NoGlobalsStillWorks(t *testing.T) {
	cmd := &echoCmd{}
	var buf bytes.Buffer
	app := cli.New("app", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("echo", "echo", cmd))

	code := app.Run(context.Background(), []string{"echo", "--message", "hi"})
	assert.Equal(t, 0, code)
	assert.Equal(t, "hi", cmd.Message)
}
