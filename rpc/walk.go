package rpc

import (
	"fmt"
	"strings"

	"github.com/gorilla/mux"
)

func Walk(r *mux.Router) {
	err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, err := route.GetPathTemplate()
		if err != nil {
			fmt.Println("Path template err:", err)
		}
		routeName := route.GetName()
		if routeName == "" {
			routeName = "<noname>"
		}
		// pathRegexp, err := route.GetPathRegexp()
		// if err != nil {
		// 	fmt.Println("Path regexp err:", err)
		// }
		methods, err := route.GetMethods()
		if err != nil {
			if strings.Contains(err.Error(), "doesn't have methods") {
				methods = []string{"ANY"}
			} else {
				fmt.Println("Methods err:", err)
			}
		}
		handler := route.GetHandler()
		for _, method := range methods {
			fmt.Printf("%s %s `%s` %s\n", method, pathTemplate, routeName, handler)
		}
		return nil
	})
	if err != nil {
		fmt.Println(err)
	}
}
