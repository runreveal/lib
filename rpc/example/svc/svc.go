package svc

import (
	"context"
	"errors"

	"github.com/runreveal/lib/rpc"
)

type Details struct {
	City string `json:"city"`
	Age  int    `json:"age"`
}

type EchoRequest struct {
	Name    string   `json:"name"`
	Message string   `json:"message"`
	Tags    []string `json:"tags"`
	Nested  Details  `json:"nested"`
}

func (r EchoRequest) Validate() error {
	if r.Message == "" {
		return errors.New("no message")
	}
	return nil
}

type EchoResponse struct {
	Message string `json:"message"`
}

func Echo(ctx context.Context, req EchoRequest) (EchoResponse, error) {
	if req.Message == "" {
		return EchoResponse{}, rpc.UserErr(errors.New("no message"))
	}
	return EchoResponse{Message: req.Message}, nil
}

func EchoCreate(ctx context.Context, req EchoRequest) (EchoResponse, error) {
	if req.Message == "" {
		return EchoResponse{}, rpc.UserErr(errors.New("no message"))
	}
	return EchoResponse{Message: req.Message}, nil
}
