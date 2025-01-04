package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"reflect"
	"strings"

	"github.com/go-playground/form/v4"
)

var schemaDecoder *form.Decoder

func init() {
	schemaDecoder = form.NewDecoder()
	schemaDecoder.SetTagName("json")
}

type call struct {
	stringFunc func() string
	handler    http.HandlerFunc
	template   Template

	// I have no idea how authz and audit hooks should look.
	// We don't have user info in the rpc package context.
	//     I don't think we should assume anything about how an application
	//     models it's users.
	// They probably want the http request which they can get from the context.
	// I think it is good to pass in the deserialized object.
	// Debating if we should get and pass the route name to the hook, but that's
	// on the request too.  Doing so would tightly couple us to a router
	// implementation.
	prehook  func(ctx context.Context, reqObj any) error
	posthook func(ctx context.Context, reqObj, respObj any, err error) error
}

type RPCOption func(*call)

func WithPreHook(hook func(ctx context.Context, reqObj any) error) RPCOption {
	return func(c *call) {
		c.prehook = hook
	}
}

func WithPostHook(hook func(context.Context, any, any, error) error) RPCOption {
	return func(c *call) {
		c.posthook = hook
	}
}

func (c call) String() string {
	return c.stringFunc()
}

func (c call) Template() Template {
	return c.template
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

	ft := reflect.ValueOf(callme).Type()
	reqT := structToTypeDef(req)
	reqName := structToTypeName(req)
	respT := structToTypeDef(resp)
	respName := structToTypeName(resp)
	c := call{
		stringFunc: func() string {
			return fmt.Sprintf("%s\n\t%v\n\t%v", ft, reqT, respT)
		},
		template: Template{
			MethodName:   ft.Name(),
			RequestType:  reqName,
			ResponseType: respName,
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
		var rq = new(Rq)

		var err error

		switch r.Method {
		case http.MethodGet, http.MethodHead:
			// Deserialize query parameters
			err = schemaDecoder.Decode(rq, r.URL.Query())
			if err != nil {
				handleErr(ctx, rw, err)
				return
			}

		default:
			hdr := r.Header.Get("Content-Type")

			switch hdr {
			case "application/x-www-form-urlencoded":
				// if type of Rq is a map or slice, don't support this method
				if reflect.TypeOf(*rq).Kind() == reflect.Slice {
					handleErr(ctx, rw, errors.New("form decoding not supported for slices"))
					return
				}

				if reflect.TypeOf(*rq).Kind() == reflect.Map {
					handleErr(ctx, rw, errors.New("form decoding not supported for maps"))
					return
				}

				// ParseForm is a no-op if the content-type is not application/x-www-form-urlencoded
				err = r.ParseForm()
				if err != nil {
					handleErr(ctx, rw, err)
					return
				}

				// Body Form Values > URL query parameters
				// if ParseForm is a no-op, r.Form is non-nil but empty
				err = schemaDecoder.Decode(rq, r.Form)
				if err != nil {
					handleErr(ctx, rw, err)
					return
				}

			case "application/json":
				err = json.NewDecoder(r.Body).Decode(rq)
				if err != nil && !errors.Is(err, io.EOF) {
					handleErr(ctx, rw, err)
					return
				}
			}
		}

		// Do Validation
		switch v := any(*rq).(type) {
		case ValidatorContext:
			if err := v.Validate(ctx); err != nil {
				handleErr(ctx, rw, err)
				return
			}
		case Validator:
			if err := v.Validate(); err != nil {
				handleErr(ctx, rw, err)
				return
			}
		}

		// pre call hook
		if c.prehook != nil {
			if err := c.prehook(ctx, *rq); err != nil {
				handleErr(ctx, rw, err)
				return
			}
		}

		// Call the procedure
		resp, err := callme(ctx, *rq)

		// post call hook
		if c.posthook != nil {
			if e2 := c.posthook(ctx, *rq, resp, err); e2 != nil {
				slog.Warn("posthook failed")
			}
		}

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

func printFunctionSignature(fn any) string { //nolint:golint,unused
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

func joinWithCommas(slice []string) string { //nolint:golint,unused
	returnString := ""
	for i, s := range slice {
		returnString += s
		if i < len(slice)-1 {
			returnString += ", "
		}
	}
	return returnString
}

func structToTypeName(s any) string {
	typ := reflect.TypeOf(s)

	switch typ.Kind() {
	case reflect.Struct:
		typeName := typ.Name()
		if typeName == "" {
			typeName = "struct{}"
		}
		pkgPath := typ.PkgPath()
		if pkgPath != "" {
			last := path.Base(pkgPath)
			typeName = last + "." + typeName
		}
		return typeName
	case reflect.Ptr:
		return "*" + structToTypeName(reflect.New(typ.Elem()).Elem().Interface())
	default:
		return typ.String() // For basic types, just return the type name
	}
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
	case reflect.Ptr:
		return "*" + structToTypeDef(reflect.New(typ.Elem()).Elem().Interface())
	default:
		return typ.String() // For basic types, just return the type name
	}
}
