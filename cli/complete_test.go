package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/runreveal/lib/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- completion test helpers ---

type serveHandler struct {
	Addr    string `cli:"addr,a"    usage:"listen address" default:":8080"`
	Verbose bool   `cli:"verbose,v" usage:"be verbose"`
}

func (s *serveHandler) Run(_ context.Context, _ []string) error {
	return nil
}

type migrateHandler struct {
	DryRun bool `cli:"dry-run" usage:"print migrations without running"`
}

func (m *migrateHandler) Run(_ context.Context, _ []string) error {
	return nil
}

type completingHandler struct {
	Name string `cli:"name" usage:"resource name"`
}

func (c *completingHandler) Run(_ context.Context, _ []string) error {
	return nil
}

func (c *completingHandler) Complete(
	_ context.Context, _ []string,
) []cli.Completion {
	return []cli.Completion{
		{Value: "alpha", Description: "first resource"},
		{Value: "beta", Description: "second resource"},
		{Value: "gamma", Description: "third resource"},
	}
}

type completeTestGlobals struct {
	Config string `cli:"config,c" usage:"config file path"`
	Debug  bool   `cli:"debug,d"  usage:"enable debug"`
}

func newTestApp(buf *bytes.Buffer) *cli.App {
	globals := &completeTestGlobals{}
	app := cli.New("testapp", "A test application",
		cli.WithGlobals(globals),
		cli.WithOutput(buf),
	)
	app.AddCommand(
		cli.Command("serve", "Start the server", &serveHandler{}),
		cli.Command("get", "Get a resource", &completingHandler{}),
		cli.Group("admin", "Administrative commands",
			cli.Command(
				"migrate", "Run database migrations",
				&migrateHandler{},
			),
		),
	)
	return app
}

func TestComplete(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantValues []string
		wantAbsent []string
	}{
		{
			name:       "top-level subcommands",
			args:       []string{"__complete", ""},
			wantValues: []string{"serve", "admin", "get"},
		},
		{
			name:       "partial subcommand match",
			args:       []string{"__complete", "se"},
			wantValues: []string{"serve"},
			wantAbsent: []string{"admin", "get"},
		},
		{
			name:       "flag completion for command",
			args:       []string{"__complete", "serve", "--"},
			wantValues: []string{"--addr", "--verbose", "--help"},
		},
		{
			name:       "flag completion with prefix",
			args:       []string{"__complete", "serve", "--a"},
			wantValues: []string{"--addr"},
			wantAbsent: []string{"--verbose"},
		},
		{
			name:       "global flags included in command flags",
			args:       []string{"__complete", "serve", "--"},
			wantValues: []string{"--config", "--debug"},
		},
		{
			name:       "nested command completion",
			args:       []string{"__complete", "admin", "mi"},
			wantValues: []string{"migrate"},
		},
		{
			name:       "nested command flag completion",
			args:       []string{"__complete", "admin", "migrate", "--"},
			wantValues: []string{"--dry-run", "--help"},
		},
		{
			name:       "completer interface for positional args",
			args:       []string{"__complete", "get", "al"},
			wantValues: []string{"alpha"},
			wantAbsent: []string{"beta", "gamma"},
		},
		{
			name:       "completer returns all matches on empty input",
			args:       []string{"__complete", "get", ""},
			wantValues: []string{"alpha", "beta", "gamma"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			app := newTestApp(&buf)

			code := app.Run(context.Background(), tt.args)
			assert.Equal(t, 0, code)

			output := buf.String()
			for _, want := range tt.wantValues {
				assert.True(
					t,
					strings.Contains(output, want),
					"expected %q in output:\n%s", want, output,
				)
			}
			for _, absent := range tt.wantAbsent {
				assert.False(
					t,
					strings.Contains(output, absent),
					"did not expect %q in output:\n%s",
					absent, output,
				)
			}
		})
	}
}

func TestCompletionScripts(t *testing.T) {
	shells := []string{"bash", "zsh", "fish"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			var buf bytes.Buffer
			app := newTestApp(&buf)

			code := app.Run(
				context.Background(),
				[]string{"completion", shell},
			)
			assert.Equal(t, 0, code)

			output := buf.String()
			require.NotEmpty(t, output)
			// The script should reference the actual binary name,
			// not the app's logical name.
			assert.NotContains(t, output, "testapp")
		})
	}
}

func TestCompletionScriptInvalidShell(t *testing.T) {
	var buf bytes.Buffer
	app := newTestApp(&buf)

	code := app.Run(
		context.Background(),
		[]string{"completion", "powershell"},
	)
	assert.Equal(t, 1, code)
}

func TestCompletionScriptNoArg(t *testing.T) {
	var buf bytes.Buffer
	app := newTestApp(&buf)

	code := app.Run(
		context.Background(),
		[]string{"completion"},
	)
	assert.Equal(t, 1, code)
}

func TestCompletionCommandsNotInHelp(t *testing.T) {
	var buf bytes.Buffer
	app := newTestApp(&buf)

	// Running with no args should show help, which should not
	// mention "completion" or "__complete".
	app.Run(context.Background(), nil)
	output := buf.String()
	assert.NotContains(t, output, "completion")
	assert.NotContains(t, output, "__complete")
}

func TestCompleteOutputFormat(t *testing.T) {
	var buf bytes.Buffer
	app := newTestApp(&buf)

	app.Run(context.Background(), []string{"__complete", ""})
	output := buf.String()

	// Each line should have a tab separator for description.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		assert.Contains(
			t, line, "\t",
			"expected tab-separated format in line: %s", line,
		)
	}
}
