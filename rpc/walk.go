package rpc

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
)

// PrintOptions holds all configuration options for PrintRoutes
type PrintOptions struct {
	methodFilter   *regexp.Regexp
	pathFilter     *regexp.Regexp
	nameFilter     *regexp.Regexp
	includeHandler bool
}

// Option defines the functional option type for configuring PrintRoutes
type Option func(*PrintOptions)

// WithMethodFilter adds a regex filter for HTTP methods
func WithMethodFilter(pattern string) Option {
	return func(o *PrintOptions) {
		if pattern != "" {
			o.methodFilter = regexp.MustCompile(pattern)
		}
	}
}

// WithPathFilter adds a regex filter for route paths
func WithPathFilter(pattern string) Option {
	return func(o *PrintOptions) {
		if pattern != "" {
			o.pathFilter = regexp.MustCompile(pattern)
		}
	}
}

// WithNameFilter adds a regex filter for route names
func WithNameFilter(pattern string) Option {
	return func(o *PrintOptions) {
		if pattern != "" {
			o.nameFilter = regexp.MustCompile(pattern)
		}
	}
}

// WithHandlerInfo configures whether handler information should be included
func WithHandlerInfo(include bool) Option {
	return func(o *PrintOptions) {
		o.includeHandler = include
	}
}

// defaultOptions returns the default configuration
func defaultOptions() *PrintOptions {
	return &PrintOptions{
		includeHandler: false,
	}
}

// PrintRoutes prints all routes in the router that match the given filters
func PrintRoutes(r *mux.Router, w io.Writer, opts ...Option) error {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	return r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err != nil {
			fmt.Fprintln(w, "Path template err:", err)
			return nil
		}

		// Apply path filter
		if options.pathFilter != nil && !options.pathFilter.MatchString(pathTemplate) {
			return nil
		}

		routeName := route.GetName()
		if routeName == "" {
			routeName = "<noname>"
		}

		// Apply name filter
		if options.nameFilter != nil && !options.nameFilter.MatchString(routeName) {
			return nil
		}

		methods, err := route.GetMethods()
		if err != nil {
			if strings.Contains(err.Error(), "doesn't have methods") {
				methods = []string{"ANY"}
			} else {
				fmt.Fprintln(w, "Methods err:", err)
				return nil
			}
		}

		handler := route.GetHandler()
		if options.includeHandler {
			for {
				if h, ok := handler.(interface{ GetHandler() http.Handler }); ok {
					handler = h.GetHandler()
				} else {
					break
				}
			}
		}

		for _, method := range methods {
			// Apply method filter and OPTIONS skip
			if (method == "OPTIONS") ||
				(options.methodFilter != nil && !options.methodFilter.MatchString(method)) {
				continue
			}

			if options.includeHandler {
				fmt.Fprintf(w, "%s %s `%s`\n\t%s\n",
					method, pathTemplate, routeName, handler)
			} else {
				fmt.Fprintf(w, "%s %s `%s`\n",
					method, pathTemplate, routeName)
			}
		}
		return nil
	})
}

type Template struct {
	RouteMethod string
	RoutePath   string

	MethodName   string
	RequestType  string
	ResponseType string
}

func Codegen(r *mux.Router, w io.Writer) {

	tmpl, err := template.New("rpc").Parse(tmpl)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}

	err = r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err != nil {
			fmt.Fprintln(w, "Path template err:", err)
		}

		methods, err := route.GetMethods()
		if err != nil {
			if strings.Contains(err.Error(), "doesn't have methods") {
				methods = []string{"ANY"}
			} else {
				fmt.Fprintln(w, "Methods err:", err)
			}
		}

		handler := route.GetHandler()
		for {
			if h, ok := handler.(interface{ GetHandler() http.Handler }); ok {
				handler = h.GetHandler()
			} else {
				break
			}
		}

		var tpl Template
		if t, ok := handler.(interface {
			Template() Template
		}); ok {
			tpl = t.Template()
		} else {
			return nil
		}

		for _, method := range methods {
			if method == "OPTIONS" {
				continue
			}
			tpl.RouteMethod = method
			tpl.RoutePath = pathTemplate
			tpl.MethodName = convertPathToName(pathTemplate)

			err := tmpl.Execute(w, tpl)
			if err != nil {
				fmt.Fprintln(w, err)
			}

			// fmt.Fprintf(w, "%s %s `%s`\n"+
			// 	"	%s\n", method, pathTemplate, routeName, tpl)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(w, err)
	}
}

// ConvertPathToName converts a path to a name, removing the slashes and making
// the first letter of each word uppercase, it also removes dashes and
// concatenates the words in camelcase as well.
func convertPathToName(path string) string {
	path = strings.Trim(path, "/")
	words := strings.Split(path, "/")
	for i, word := range words {
		words[i] = strings.Title(word) //nolint:staticcheck
	}
	return strings.Join(words, "")
}

var tmpl = `
func (c *Client) {{ .MethodName }}(ctx context.Context, req {{ .RequestType }}) ({{ .ResponseType }}, error) {
	rb := c.newReq().
		Path("{{ .RoutePath }}").
		Method("{{ .RouteMethod }}").
		BodyJSON(req)

	req, err := rb.Request(ctx)
	if err != nil {
		return nil, fmt.Errorf("runreveal client: %w", err)
	}
	var resp {{ .ResponseType }}
	err = c.do(req, &resp)
	return &resp, err
}

`
