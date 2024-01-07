package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/runreveal/runreveal/types"
)

// type call struct {
// }

// type RPCOption func(*call)

func RPC[Rq, Rp any](
	callme func(ctx context.Context, rq Rq) (rp Rp, err error),
	// opts ...RPCOption,
) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Upgrade the context
		ctx := r.Context()
		ctx = context.WithValue(ctx, reqContextKey, r)
		rw := &responseWrapper{wrapped: w}
		ctx = context.WithValue(ctx, respContextKey, rw)

		// deserialize request
		var rq Rq
		err := json.NewDecoder(r.Body).Decode(&rq)
		if err != nil {
			handleErr(ctx, rw, err)
			return
		}

		resp, err := callme(ctx, rq)

		if err != nil {
			handleErr(ctx, rw, err)
			return
		}

		if rw.status != 0 {
			slog.Warn("response sent in wrapped rpc call")
			return
		}
		// serialize response
		success(rw, resp)
	})
}

type RPCResponse struct {
	Success bool   `json:"success"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Success is the successful path for writing responses to the client
func success(rw *responseWrapper, result any) {
	buf, err := json.Marshal(types.APIResponse{
		Success: true,
		Result:  result,
	})
	if err != nil {
		slog.Warn(fmt.Sprintf("error encoding: %+v", err))
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = rw.Write(buf)
	if err != nil {
		slog.Error(fmt.Sprintf("writing resp: %+v", err))
	}
	_, err = rw.Write([]byte("\n"))
	if err != nil {
		slog.Error(fmt.Sprintf("writing resp: %v", err))
	}
}
