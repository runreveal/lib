package await

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"log/slog"
)

type Runner interface {
	Run(context.Context) error
}

type RunFunc func(context.Context) error

func (rf RunFunc) Run(ctx context.Context) error {
	return rf(ctx)
}

type runner struct {
	funcs        []RunFunc
	funcNames    []string
	withSignals  bool
	startMu      sync.Mutex
	started      bool
	stopTimeout  time.Duration
	proceedOnNil bool
}

type Option func(*runner)

func WithSignals(r *runner) {
	r.withSignals = true
}

func WithStopTimeout(d time.Duration) Option {
	return func(r *runner) {
		r.stopTimeout = d
	}
}

// WithContinueOnNil will allow the runner to continue if a functions returns a
// nil error. This is useful for running jobs that have a fixed lifetime and
// you want to run all of them, but still cancel and report if errors are returned
// which aren't nil
func WithContinueOnNil(r *runner) {
	r.proceedOnNil = true
}

func New(opts ...Option) *runner {
	r := &runner{
		funcs: make([]RunFunc, 0),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *runner) Add(f Runner) {
	r.startMu.Lock()
	if r.started {
		panic("Add called after Run started")
	}
	r.funcs = append(r.funcs, f.Run)
	r.funcNames = append(r.funcNames, "")
	r.startMu.Unlock()
}

func (r *runner) AddNamed(f Runner, name string) {
	r.startMu.Lock()
	if r.started {
		panic("Add called after Run started")
	}
	r.funcs = append(r.funcs, f.Run)
	r.funcNames = append(r.funcNames, name)
	r.startMu.Unlock()
}

// RunAll runs all the given synchronous functions and returns the first
// error returned by any of them, with the exception of context.Canceled. It
// catches SIGINT and SIGTERM and begins shutdown by canceling the context.
// It also cancels the other contexts in case it encounters an error, and waits
// until they've returned before returning itself.
func (r *runner) Run(ctx context.Context) error {
	r.startMu.Lock()
	if r.started {
		panic("Run called twice")
	}
	r.started = true
	r.startMu.Unlock()

	if r.stopTimeout <= 0 {
		r.stopTimeout = 10 * time.Second
	}

	errc := make(chan error, len(r.funcs))
	// this cancel func cancels all subroutines
	subctx, cancel := context.WithCancelCause(ctx)

	var waitCount int32

	for i, f := range r.funcs {
		atomic.AddInt32(&waitCount, 1)
		go func(fn func(context.Context) error, idx int) {
			err := fn(subctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				if r.funcNames[idx] != "" {
					slog.Info(fmt.Sprintf("subroutine %s error: %+v", r.funcNames[idx], err))
				} else {
					slog.Info(fmt.Sprintf("subroutine error: %+v", err))
				}
			}
			// Order matters for the following two statements.
			// We must decrement before writing to the channel so that waitCount is
			// accurate when we read the remaining waitCount below after reading errC.
			atomic.AddInt32(&waitCount, -1)
			errc <- err
		}(f, i)
	}

	var err error
	var sigc chan os.Signal

	if r.withSignals {
		// receive from a nil channel blocks forever. so by wrapping the allocation
		// in this statement, we're only making the channel non-nil if signals are
		// enabled. Select below will then only have the option between ctx.Done or
		// the err channel
		sigc = make(chan os.Signal, 1)
		signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	}

loop:
	select {
	case sig := <-sigc:
		slog.Error("stopping on signal", "signal", sig)
	case <-subctx.Done():
		err = subctx.Err()
		if !errors.Is(err, context.Canceled) {
			slog.Error("error on context done", "err", err)
		}
	case err = <-errc:
		if err != nil {
			slog.Warn("await: stopping on error returned", "err", err)
		} else {
			if r.proceedOnNil && atomic.LoadInt32(&waitCount) > 0 {
				goto loop
			} else {
				slog.Debug("await: stopping on subroutine(s) complete")
			}
		}
	}

	cancel(fmt.Errorf("await: %w", err))

	err = waitOrTimeout(r.stopTimeout, &waitCount, err)
	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}

// waitTimeout will return either when the context is canceled or when the
// counter reaches 0. It will check the counter every 10ms.
func waitOrTimeout(timeout time.Duration, counter *int32, err error) error {
	// slog.Info(fmt.Sprintf("await: waiting %s for subroutines to finish", timeout))
	for i := 0 * time.Second; i < timeout; i += 100 * time.Millisecond {
		if atomic.LoadInt32(counter) == 0 {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("await: shutdown timeout: %w", err)
	}
	return nil
}

// ListenAndServe provides a graceful shutdown for an http.Server.
// usage: `w.Add(await.ListenAndServe(srv))` followed by the normal w.Run(ctx)
func ListenAndServe(server *http.Server) Runner {
	return RunFunc(func(ctx context.Context) error {
		errc := make(chan error, 1)
		go func() {
			errc <- server.ListenAndServe()
		}()

		select {
		case <-ctx.Done():
			cto, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := server.Shutdown(cto)
			if err != nil {
				return err
			}
			return ctx.Err()
		case err := <-errc:
			return err
		}
	})
}
