package sub

import (
	"context"
	"errors"

	"github.com/runreveal/lib/rpc"
)

type EchoRequest struct {
	Message string `json:"message"`
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
