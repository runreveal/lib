package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/runreveal/lib/await"
)

type EchoRequest struct {
	Message string `json:"message"`
}

type EchoResponse struct {
	Message string `json:"message"`
}

func Echo(ctx context.Context, req EchoRequest) (EchoResponse, error) {
	if req.Message == "" {
		return EchoResponse{}, UserErr(errors.New("no message"))
	}
	return EchoResponse{Message: req.Message}, nil
}

func main() {
	router := mux.NewRouter()
	router.Handle("/echo", RPC(Echo))

	s := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	w := await.New(await.WithSignals)
	w.Add(await.ListenAndServe(s))
	err := w.Run(context.Background())
	if err != nil {
		slog.Error("error running server", "error", err)
	}
}

// func upgradeContext(w http.ResponseWriter, r *http.Request) context.Context {
// 	ctx := r.Context()

// }

// func (c call) deserialize(r *http.Request, obj any) error {
// 	// Read and validate headers
// 	// pick serialization format based on headers
// }
