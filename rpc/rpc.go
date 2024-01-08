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
}

type RPCOption func(*call)

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
		rw := &responseWrapper{wrapped: w}
		ctx = context.WithValue(ctx, respContextKey, rw)

		// deserialize request
		var rq Rq
		err := json.NewDecoder(r.Body).Decode(&rq)
		if err != nil {
			handleErr(ctx, rw, err)
			return
		}

		if v, ok := any(rq).(interface{ Validate() error }); ok {
			if err := v.Validate(); err != nil {
				handleErr(ctx, rw, err)
				return
			}
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
