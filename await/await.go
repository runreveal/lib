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

	"golang.org/x/exp/slog"
)

type RunFunc func(context.Context) error

type runner struct {
	funcs       []RunFunc
	withSignals bool
	startMu     sync.Mutex
	started     bool
}

type Option func(*runner)

func WithSignals(r *runner) {
	r.withSignals = true
}

func New(...Option) *runner {
	return &runner{
		funcs: make([]RunFunc, 0),
	}
}

func (r *runner) Add(f RunFunc) {
	r.startMu.Lock()
	if r.started {
		panic("Add called after Run started")
	}
	r.funcs = append(r.funcs, f)
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

	errc := make(chan error, len(r.funcs))
	ctx, cancel := context.WithCancel(ctx)

	var waitCount int32

	defer func() {
		cancel()
		cc, cncl := context.WithTimeout(context.Background(), 10*time.Second)
		waitTimeout(cc, &waitCount)
		cncl()
	}()

	for _, f := range r.funcs {
		atomic.AddInt32(&waitCount, 1)
		go func(fn func(context.Context) error) {
			err := fn(ctx)
			slog.Debug(fmt.Sprintf("subroutine error: %+v", err))
			errc <- err
			atomic.AddInt32(&waitCount, -1)
		}(f)
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

	select {
	case sig := <-sigc:
		slog.Error("stopping on signal", "signal", sig)
	case <-ctx.Done():
		err = ctx.Err()
		if errors.Is(err, context.Canceled) {
			return nil
		}
	case err = <-errc:
	}

	return err
}

func waitTimeout(ctx context.Context, counter *int32) {
	ticker := time.NewTicker(10 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if atomic.LoadInt32(counter) == 0 {
				return
			}
		}
	}
}

func ListenAndServe(server *http.Server) func(context.Context) error {
	return func(ctx context.Context) error {
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
	}
}
