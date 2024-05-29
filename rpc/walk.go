package rpc

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

func PrintRoutes(r *mux.Router, w io.Writer) {
	err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err != nil {
			fmt.Fprintln(w, "Path template err:", err)
		}
		routeName := route.GetName()
		if routeName == "" {
			routeName = "<noname>"
		}
		// pathRegexp, err := route.GetPathRegexp()
		// if err != nil {
		// 	fmt.Fprintln("Path regexp err:", err)
		// }

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
		for _, method := range methods {
			if method == "OPTIONS" {
				continue
			}
			fmt.Fprintf(w, "%s %s `%s`\n"+
				"	%s\n", method, pathTemplate, routeName, handler)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(w, err)
	}
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
		routeName := route.GetName()
		if routeName == "" {
			routeName = "<noname>"
		}
		// pathRegexp, err := route.GetPathRegexp()
		// if err != nil {
		// 	fmt.Fprintln("Path regexp err:", err)
		// }

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
		words[i] = strings.Title(word)
	}
	return strings.Join(words, "")
}

var tmpl = `

func (c *Client) {{ .MethodName }}(ctx context.Context, req types.{{ .RequestType }}) (*types.{{ .ResponseType }}, error) {
	rb := c.newReq().
		Path("{{ .RoutePath }}").
		Method("{{ .RouteMethod }}").
		BodyJSON(req)

	req, err := rb.Request(ctx)
	if err != nil {
		return nil, fmt.Errorf("runreveal client: %w", err)
	}
	var resp types.{{ .ResponseType }}
	err = c.do(req, &resp)
	return &resp, err
}

`
