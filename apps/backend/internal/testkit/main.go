package testkit

import (
	"context"
	"fmt"
	"os"
	"testing"

	"go.uber.org/goleak"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

type MainOption func(*mainOptions)

type mainOptions struct {
	noDB  bool
	nats  bool
	child func()
	setup func() (func(), error)
}

func WithNATS() MainOption {
	return func(o *mainOptions) { o.nats = true }
}

func NoDB() MainOption {
	return func(o *mainOptions) { o.noDB = true }
}

func WithChild(main func()) MainOption {
	return func(o *mainOptions) { o.child = main }
}

func WithSetup(setup func() (cleanup func(), err error)) MainOption {
	return func(o *mainOptions) { o.setup = setup }
}

func Main(m *testing.M, opts ...MainOption) {
	var o mainOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.child != nil {
		childMain(o.child)
	}
	r := &runThenClose{m: m}
	if err := r.start(o); err != nil {
		r.close()
		_, _ = fmt.Fprintf(os.Stderr, "testkit.Main: %v\n", err)
		os.Exit(1)
	}
	goleak.VerifyTestMain(r)
}

type runThenClose struct {
	m       *testing.M
	s       *server
	nats    *natsServer
	cleanup func()
}

func (r *runThenClose) start(o mainOptions) error {
	if o.setup != nil {
		cleanup, err := o.setup()
		if err != nil {
			return err
		}
		r.cleanup = cleanup
	}
	if !o.noDB {
		s, err := open(context.Background(), config.TestDBURL(os.Environ()))
		if err != nil {
			return err
		}
		r.s = s
		current.Store(s)
	}
	if o.nats {
		ns, err := startNATS()
		if err != nil {
			return err
		}
		r.nats = ns
		natsCurrent.Store(ns)
	}
	return nil
}

func (r *runThenClose) Run() int {
	code := r.m.Run()
	r.close()
	return code
}

func (r *runThenClose) close() {
	if r.nats != nil {
		r.nats.stop()
	}
	if r.s != nil {
		if r.s.holder != nil {
			_ = r.s.holder.Close(context.Background())
		}
		r.s.admin.Close()
	}
	if r.cleanup != nil {
		r.cleanup()
	}
}
