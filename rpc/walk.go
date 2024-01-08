package rpc

import (
	"fmt"
	"io"
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
		for _, method := range methods {
			if method == "OPTIONS" {
				continue
			}
			fmt.Fprintf(w, "%s %s `%s` %s\n", method, pathTemplate, routeName, handler)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(w, err)
	}
}
