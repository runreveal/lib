package rpc

import "context"

type ValidatorContext interface {
	Validate(context.Context) error
}

type Validator interface {
	Validate() error
}
