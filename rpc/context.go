package rpc

import (
	"context"
	"net/http"
)

type contextKey struct{ name string }

var (
	reqContextKey  = contextKey{name: "requestKey"}
	respContextKey = contextKey{name: "responseKey"}
)

func Request(ctx context.Context) *http.Request {
	v, ok := ctx.Value(reqContextKey).(*http.Request)
	if !ok {
		panic("request not set on context. ensure handler is wrapped")
	}
	return v
}

func ResponseWriter(ctx context.Context) http.ResponseWriter {
	v, ok := ctx.Value(respContextKey).(*responseWrapper)
	if !ok {
		panic("response not set on context. ensure handler is wrapped")
	}
	return v
}
