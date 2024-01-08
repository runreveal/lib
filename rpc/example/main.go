package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/runreveal/lib/await"
	"github.com/runreveal/lib/rpc"
	"github.com/runreveal/lib/rpc/example/sub"
	"github.com/runreveal/lib/rpc/example/svc"
)

func main() {
	router := mux.NewRouter()
	router.Handle("/echo/{id}", rpc.RPC(svc.Echo)).Methods("GET").Name("echo#getByID")
	router.Handle("/echo/create", rpc.RPC(svc.EchoCreate)).Methods("POST").Name("echo#create")
	router.Handle("/echo2", rpc.RPC(sub.Echo))

	s := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	rpc.PrintRoutes(router, os.Stdout)

	w := await.New(await.WithSignals)
	w.Add(await.ListenAndServe(s))
	err := w.Run(context.Background())
	if err != nil {
		slog.Error("error running server", "error", err)
	}
}
