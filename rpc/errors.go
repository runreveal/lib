package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"
)

type CustomError interface {
	error
	Status() int
	Format(context.Context) any
}

var (
	errorRegistry = []CustomError{}
)

func RegisterErrorHandler(ce CustomError) {
	fmt.Printf("registering error handler: %p, %I, %v\n", &ce, ce, ce)
	errorRegistry = append(errorRegistry, ce)
}

// HandleErr is equivlant to errResp but using more flexible error types
func handleErr(ctx context.Context, w *responseWrapper, err error) {
	errorHelper(fmt.Sprintf("error: %+v\n", err))

	if w.status != 0 {
		slog.Error("response sent before error was handled")
		return
	}

	e := json.NewEncoder(w)
	// Custom errors take precedence over default errors
	for _, ce := range errorRegistry {
		if errors.As(err, &ce) {
			w.WriteHeader(ce.Status())
			encErr := e.Encode(ce.Format(ctx))
			if encErr != nil {
				errorHelper(fmt.Sprintf("error encountered encoding error response: %v", encErr))
			}
			return
		}
	}

	var (
		ue     UserError
		ae     AuthError
		le     LimitError
		encErr error
	)
	switch {
	case errors.As(err, &ae):
		w.WriteHeader(http.StatusUnauthorized)
		encErr = e.Encode(RPCResponse{Error: ae.AuthError()})

	case errors.As(err, &ue):
		w.WriteHeader(http.StatusBadRequest)
		encErr = e.Encode(RPCResponse{Error: ue.UserError()})

	case errors.Is(err, context.Canceled):
		w.WriteHeader(http.StatusServiceUnavailable)
		encErr = e.Encode(RPCResponse{Error: "context canceled"})

	case errors.Is(err, sql.ErrNoRows):
		w.WriteHeader(http.StatusBadRequest)
		encErr = e.Encode(RPCResponse{Error: "no record could be found"})

	case errors.As(err, &le):
		w.WriteHeader(http.StatusUpgradeRequired)
		encErr = e.Encode(RPCResponse{Error: le.LimitError()})

	default:
		w.WriteHeader(http.StatusInternalServerError)
		encErr = e.Encode(RPCResponse{Error: "unknown error"})
	}
	if encErr != nil {
		errorHelper(fmt.Sprintf("error encountered encoding error response: %v", encErr))
	}
}

type UserError interface {
	UserError() string
}

type userErr struct {
	err error
}

func UserErr(err error) error {
	if err == nil {
		return nil
	}
	return userErr{
		err: err,
	}
}

func (ue userErr) UserError() string {
	return ue.err.Error()
}

func (ue userErr) Error() string {
	return ue.err.Error()
}

//////////////////////

type AuthError interface {
	AuthError() string
}

type authErr struct {
	err error
}

func AuthErr(err error) error {
	if err == nil {
		return nil
	}
	return authErr{
		err: err,
	}
}

func (ae authErr) AuthError() string {
	return ae.err.Error()
}

func (ae authErr) Error() string {
	return ae.err.Error()
}

//////////////////

type LimitError interface {
	LimitError() string
}

type limitErr struct {
	err error
}

func LimitErr(err error) error {
	if err == nil {
		return nil
	}
	return limitErr{
		err: err,
	}
}

func (le limitErr) Error() string {
	return le.err.Error()
}

func (le limitErr) LimitError() string {
	return le.err.Error()
}

var (
	ErrLimitReached = AuthErr(errors.New("limit reached"))
)

type ErrVersionMismatch struct {
	Err           error
	ClientVersion string
	ServerVersion string
}

func (e ErrVersionMismatch) Error() string {
	// Don't mask the wrapped error if it's rewrapped
	return e.Err.Error()
}

func (e ErrVersionMismatch) Warning() string {
	return fmt.Sprintf("client/server version mismatch (c: %s s: %s).", e.ClientVersion, e.ServerVersion)
}

func (e ErrVersionMismatch) Unwrap() error {
	return e.Err
}

// errorHelper is a helper function to log errors to the default logger and
// include the file and line number of the function calling HandleErr
// we may want to inject the logger in the future
func errorHelper(format string, args ...any) {
	l := slog.Default()
	if !l.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip [Callers, errorHelper, HandleErr]
	r := slog.NewRecord(time.Now(), slog.LevelError, fmt.Sprintf(format, args...), pcs[0])
	_ = l.Handler().Handle(context.Background(), r)
}
