package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
)

type call struct {
	stringFunc func() string
	handler    http.HandlerFunc

	// I have no idea how authz and audit hooks should look.
	// We don't have user info in the rpc package context.
	//     I don't think we should assume anything about how an application
	//     models it's users.
	// They probably want the http request which they can get from the context.
	// I think it is good to pass in the deserialized object.
	// Debating if we should get and pass the route name to the hook, but that's
	// on the request too.  Doing so would tightly couple us to a router
	// implementation.
	authzHook func(ctx context.Context, reqObj any) error
	auditHook func(ctx context.Context, reqObj any) error
}

type RPCOption func(*call)

func WithAuthzHook(hook func(ctx context.Context, reqObj any) error) RPCOption {
	return func(c *call) {
		c.authzHook = hook
	}
}

func WithAuditHook(hook func(ctx context.Context, reqObj any) error) RPCOption {
	return func(c *call) {
		c.auditHook = hook
	}
}

func (c call) String() string {
	return c.stringFunc()
}

func (c call) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.handler(w, r)
}

func RPC[Rq, Rp any](
	callme func(ctx context.Context, rq Rq) (rp Rp, err error),
	opts ...RPCOption,
) http.Handler {
	var (
		req  Rq
		resp Rp
	)

	c := call{
		stringFunc: func() string {
			ft := reflect.ValueOf(callme).Type()
			reqT := structToTypeDef(req)
			respT := structToTypeDef(resp)
			return fmt.Sprintf("%v\n\t%s\n\t%s", ft, reqT, respT)
		},
	}

	for _, opt := range opts {
		opt(&c)
	}

	c.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Upgrade the context
		ctx := r.Context()
		ctx = context.WithValue(ctx, reqContextKey, r)

		// Upgrade the responseWriter so we can know if the application has already
		// sent a response
		rw := &responseWrapper{wrapped: w}
		ctx = context.WithValue(ctx, respContextKey, rw)

		// Deserialize request
		var rq Rq
		err := json.NewDecoder(r.Body).Decode(&rq)
		if err != nil {
			handleErr(ctx, rw, err)
			return
		}

		// Do Validation
		switch v := any(rq).(type) {
		case interface{ Validate(context.Context) error }:
			if err := v.Validate(ctx); err != nil {
				handleErr(ctx, rw, err)
				return
			}
		case interface{ Validate() error }:
			if err := v.Validate(); err != nil {
				handleErr(ctx, rw, err)
				return
			}
		}

		// Authorization hook
		if c.authzHook != nil {
			if err := c.authzHook(ctx, rq); err != nil {
				handleErr(ctx, rw, err)
				return
			}
		}

		// Audit logging hook
		if c.auditHook != nil {
			if err := c.auditHook(ctx, rq); err != nil {
				handleErr(ctx, rw, err)
				return
			}
		}

		// Call the procedure
		resp, err := callme(ctx, rq)
		if err != nil {
			handleErr(ctx, rw, err)
			return
		}

		// Check if the response was already sent (application hijacked response)
		// not sure if this needs to be guarded w/ a mutex
		if rw.status != 0 {
			slog.Warn("response sent in wrapped rpc call")
			return
		}

		// serialize response
		success(rw, resp)
	})

	return c
}

type Empty struct{}

type RPCResponse struct {
	Success bool   `json:"success"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Success is the successful path for writing responses to the client
func success(rw *responseWrapper, result any) {
	buf, err := json.Marshal(struct {
		Success bool `json:"success"`
		Result  any  `json:"result,omitempty"`
	}{
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

func printFunctionSignature(fn any) string {
	// Get the type of the function
	funcType := reflect.TypeOf(fn)

	// Get and print the function's input parameters
	var params []string
	for i := 0; i < funcType.NumIn(); i++ {
		params = append(params, funcType.In(i).Name())
	}

	// Get and print the function's return values
	var returns []string
	for i := 0; i < funcType.NumOut(); i++ {
		returns = append(returns, funcType.Out(i).Name())
	}

	return fmt.Sprintf("func(%s) (%s)\n",
		joinWithCommas(params), joinWithCommas(returns))
}

func joinWithCommas(slice []string) string {
	returnString := ""
	for i, s := range slice {
		returnString += s
		if i < len(slice)-1 {
			returnString += ", "
		}
	}
	return returnString
}

func structToTypeDef(s any) string {
	typ := reflect.TypeOf(s)

	switch typ.Kind() {
	case reflect.Struct:
		var fields []string
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			fields = append(fields, field.Name+" "+field.Type.String())
		}
		typeName := typ.Name()
		if typeName == "" {
			typeName = "struct"
		}
		return typeName + " {" + strings.Join(fields, "; ") + "}"

	default:
		return typ.String() // For basic types, just return the type name
	}

	// val := reflect.ValueOf(s)
	// fmt.Println(val.Kind())
	// fmt.Println(reflect.TypeOf(s))
	// if val.Kind() != reflect.Struct {
	// 	panic("not a struct")
	// }

	// var fields []string
	// typ := val.Type()
	// for i := 0; i < val.NumField(); i++ {
	// 	field := typ.Field(i)
	// 	fields = append(fields, field.Name+" "+field.Type.String())
	// }

	// return fmt.Sprintf("%T {", s) + strings.Join(fields, "; ") + "}"
}

// func upgradeContext(w http.ResponseWriter, r *http.Request) context.Context {
// 	ctx := r.Context()

// }

// func (c call) deserialize(r *http.Request, obj any) error {
// 	// Read and validate headers
// 	// pick serialization format based on headers
// }
