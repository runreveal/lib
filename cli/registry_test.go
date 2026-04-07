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

// --- WithLong tests ---

func TestWithLong_AppearsInCommandHelp(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("up", "Launch a pod", &noopCmd{},
		cli.WithLong("Launch a new pod for the given branch.\n\nUse --issue to seed from a GitHub issue."),
	))

	code := app.Run(context.Background(), []string{"up", "--help"})
	assert.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "myapp up - Launch a pod")
	assert.Contains(t, out, "Launch a new pod for the given branch.")
	assert.Contains(t, out, "Use --issue to seed from a GitHub issue.")
}

func TestWithLong_LongTextIsIndented(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("up", "Launch a pod", &noopCmd{},
		cli.WithLong("Detailed description here."),
	))

	app.Run(context.Background(), []string{"up", "--help"})
	out := buf.String()
	// Long text should be indented with 2 spaces.
	assert.Contains(t, out, "  Detailed description here.")
}

func TestWithLong_LongTextAppearsBeforeUsage(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("up", "Launch a pod", &noopCmd{},
		cli.WithLong("Long text here."),
	))

	app.Run(context.Background(), []string{"up", "--help"})
	out := buf.String()
	longIdx := strings.Index(out, "Long text here.")
	usageIdx := strings.Index(out, "Usage:")
	require.True(t, longIdx >= 0, "long text not found")
	require.True(t, usageIdx >= 0, "Usage: not found")
	assert.Less(t, longIdx, usageIdx, "long text should appear before Usage:")
}

func TestWithLong_ShortDescStillInParentListing(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("up", "Launch a pod", &noopCmd{},
		cli.WithLong("This is a very long description."),
	))

	// App-level help shows short desc, not long text.
	app.Run(context.Background(), []string{"--help"})
	out := buf.String()
	assert.Contains(t, out, "Launch a pod")
	assert.NotContains(t, out, "This is a very long description.")
}

func TestWithLong_NoLongTextBackwardCompat(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Command("serve", "Start the server", &noopCmd{}))

	code := app.Run(context.Background(), []string{"serve", "--help"})
	assert.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "myapp serve - Start the server")
	assert.Contains(t, out, "Usage:")
}

func TestWithLong_GroupLongText(t *testing.T) {
	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Group("db", "Database commands",
		cli.WithLong("Manage database migrations and connections."),
		cli.Command("migrate", "Run migrations", &noopCmd{}),
	))

	code := app.Run(context.Background(), []string{"db"})
	assert.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "Database commands")
	assert.Contains(t, out, "Manage database migrations and connections.")
}

// --- Registry tests ---

func TestRegistry_RegisterAndRetrieve(t *testing.T) {
	before := len(cli.Registered())
	cli.Register(cli.Command("reg-test-cmd", "a registered command", &noopCmd{}))
	after := cli.Registered()
	assert.Equal(t, before+1, len(after))
}

func TestRegistry_MultipleRegisterCalls(t *testing.T) {
	before := len(cli.Registered())
	cli.Register(cli.Command("reg-multi-1", "cmd 1", &noopCmd{}))
	cli.Register(cli.Command("reg-multi-2", "cmd 2", &noopCmd{}))
	after := cli.Registered()
	assert.Equal(t, before+2, len(after))
}

func TestRegistry_ReturnsCopy(t *testing.T) {
	cli.Register(cli.Command("reg-copy-test", "cmd", &noopCmd{}))
	a := cli.Registered()
	b := cli.Registered()
	// Mutating one slice should not affect the other.
	a[0] = nil
	assert.NotNil(t, b[0])
}

func TestRegistry_IntegratesWithApp(t *testing.T) {
	cli.Register(cli.Command("reg-app-cmd", "registered app command", &noopCmd{}))

	var buf bytes.Buffer
	app := cli.New("myapp", "test", cli.WithOutput(&buf))
	app.AddCommand(cli.Registered()...)

	code := app.Run(context.Background(), []string{"--help"})
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "reg-app-cmd")
}
