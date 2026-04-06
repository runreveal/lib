// Command example demonstrates the github.com/runreveal/lib/cli framework
// with github.com/runreveal/lib/loader for polymorphic config loading.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/runreveal/lib/cli"
	"github.com/runreveal/lib/loader"
)

//go:embed config.json
var defaultConfig []byte

// ---------------------------------------------------------------------------
// Source: an interface with multiple implementations loaded via loader
// ---------------------------------------------------------------------------

type Source interface {
	Name() string
}

func init() {
	loader.Register[Source]("webhook", func() loader.Builder[Source] { return &WebhookConfig{} })
	loader.Register[Source]("syslog", func() loader.Builder[Source] { return &SyslogConfig{} })
}

// WebhookConfig is the config for a webhook source.
type WebhookConfig struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

func (w *WebhookConfig) Configure() (Source, error) {
	return &WebhookSource{path: w.Path}, nil
}

type WebhookSource struct{ path string }

func (w *WebhookSource) Name() string { return "webhook:" + w.path }

// SyslogConfig is the config for a syslog source.
type SyslogConfig struct {
	Type string `json:"type"`
	Addr string `json:"addr"`
}

func (s *SyslogConfig) Configure() (Source, error) {
	return &SyslogSource{addr: s.Addr}, nil
}

type SyslogSource struct{ addr string }

func (s *SyslogSource) Name() string { return "syslog:" + s.addr }

// ---------------------------------------------------------------------------
// Cache: a single-value polymorphic config
// ---------------------------------------------------------------------------

type Cache interface {
	Name() string
}

func init() {
	loader.Register[Cache]("memory", func() loader.Builder[Cache] { return &MemoryCacheConfig{} })
}

type MemoryCacheConfig struct {
	Type    string `json:"type"`
	MaxSize int    `json:"max_size"`
}

func (m *MemoryCacheConfig) Configure() (Cache, error) {
	return &MemoryCache{maxSize: m.MaxSize}, nil
}

type MemoryCache struct{ maxSize int }

func (m *MemoryCache) Name() string { return fmt.Sprintf("memory(max=%d)", m.maxSize) }

// ---------------------------------------------------------------------------
// AppConfig: loaded from config.json via config:"." on Globals
// ---------------------------------------------------------------------------

type AppConfig struct {
	Sources []loader.Loader[Source] `json:"sources"`
	Cache   loader.Loader[Cache]    `json:"cache"`
	Server  ServerConfig            `json:"server"`
}

type ServerConfig struct {
	Addr string `json:"addr"`
}

// ---------------------------------------------------------------------------
// Globals: shared flags + config + initialized resources
// ---------------------------------------------------------------------------

type Globals struct {
	Verbose bool      `cli:"verbose,v" usage:"enable verbose output"`
	Config  string    `cli:"config,c"  usage:"config file path"      default:"config.json"`
	Cfg     AppConfig `                                                                    config:"."`

	// Initialized resources
	sources []Source
	cache   Cache
}

func (g *Globals) Configure() error {
	for _, src := range g.Cfg.Sources {
		s, err := src.Configure()
		if err != nil {
			return fmt.Errorf("configuring source: %w", err)
		}
		g.sources = append(g.sources, s)
	}
	if g.Cfg.Cache.Builder != nil {
		c, err := g.Cfg.Cache.Configure()
		if err != nil {
			return fmt.Errorf("configuring cache: %w", err)
		}
		g.cache = c
	}
	return nil
}

func (g *Globals) Validate() error {
	if len(g.sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}
	return nil
}

// ExtraHelp implements cli.HelpExtra. It uses loader.List and
// loader.Describe to show available types and their config fields,
// bridging the cli and loader packages without either knowing
// about the other.
func (g *Globals) ExtraHelp() string {
	var b strings.Builder
	b.WriteString("\nAvailable source types:\n")
	describeLoaderTypes[Source](&b)
	b.WriteString("\nAvailable cache types:\n")
	describeLoaderTypes[Cache](&b)
	return b.String()
}

// describeLoaderTypes lists registered loader types for T with their
// config fields (derived from JSON struct tags on the builder).
func describeLoaderTypes[T any](b *strings.Builder) {
	for _, name := range loader.List[T]() {
		builder, ok := loader.Describe[T](name)
		if !ok {
			fmt.Fprintf(b, "  %s\n", name)
			continue
		}
		fields := describeFields(builder)
		if len(fields) == 0 {
			fmt.Fprintf(b, "  %s\n", name)
		} else {
			fmt.Fprintf(b, "  %-12s %s\n", name, strings.Join(fields, ", "))
		}
	}
}

// describeFields reflects on a struct's JSON tags to produce
// "name (type)" descriptions for each exported field, skipping "type".
func describeFields(v any) []string {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "type" {
			continue
		}
		out = append(out, fmt.Sprintf("%s (%s)", name, f.Type.Name()))
	}
	return out
}

// ---------------------------------------------------------------------------
// serve: uses initialized resources from Globals
// ---------------------------------------------------------------------------

type ServeCmd struct {
	Addr string `cli:"addr,a" usage:"listen address"`
}

func (s *ServeCmd) Configure() error {
	// Apply server config from file as default if --addr not set.
	return nil
}

func (s *ServeCmd) Run(ctx context.Context, args []string) error {
	g := cli.GlobalsFromContext[Globals](ctx)
	if g == nil {
		return fmt.Errorf("globals not available")
	}

	fmt.Printf("server addr: %s\n", s.Addr)
	fmt.Printf("sources:\n")
	for _, src := range g.sources {
		fmt.Printf("  - %s\n", src.Name())
	}
	if g.cache != nil {
		fmt.Printf("cache: %s\n", g.cache.Name())
	}
	return nil
}

// ---------------------------------------------------------------------------
// list-sources: shows configured sources
// ---------------------------------------------------------------------------

type ListSourcesCmd struct{}

func (l *ListSourcesCmd) Run(ctx context.Context, args []string) error {
	g := cli.GlobalsFromContext[Globals](ctx)
	if g == nil {
		return fmt.Errorf("globals not available")
	}

	for _, src := range g.sources {
		fmt.Println(src.Name())
	}
	return nil
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	globals := &Globals{}
	serveCmd := &ServeCmd{}

	app := cli.New("example", "Example CLI demonstrating cli + loader",
		cli.WithVersion("1.0.0"),
		cli.WithGlobals(globals),
		cli.WithConfigFlag("config"),
		cli.WithDefaultConfig(defaultConfig),
	)

	app.AddCommand(
		cli.Command("serve", "Start the HTTP server", serveCmd),
		cli.Command("list-sources", "List configured sources", &ListSourcesCmd{},
			cli.WithArgs(cli.NoArgs),
		),
	)

	os.Exit(app.Run(context.Background(), os.Args[1:]))
}
