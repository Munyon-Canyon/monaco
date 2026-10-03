//go:build faultpoints

package faultpoint

import (
	"context"
	"sync/atomic"
)

const Enabled = true

type armedKey struct{}

type armed struct {
	name Name
	left atomic.Int64
}

type configured struct {
	name Name
	flow string
}

var process atomic.Pointer[configured]

func ConfiguredFlow() string {
	if p := process.Load(); p != nil {
		return p.flow
	}
	return ""
}

func Hit(ctx context.Context, name Name) {
	if p := process.Load(); p != nil && p.name == name && (p.flow == "" || p.flow == Flow(ctx)) {
		panic(Crash{Name: name})
	}
	if a, ok := ctx.Value(armedKey{}).(*armed); ok && a.name == name && a.left.Add(-1) < 0 {
		panic(Crash{Name: name})
	}
}

func ArmedAfter(ctx context.Context, name Name, skip int) context.Context {
	a := &armed{name: name}
	a.left.Store(int64(skip))
	return context.WithValue(ctx, armedKey{}, a)
}

func Configure(name string) error {
	if name == "" {
		process.Store(nil)
		return nil
	}
	point, flow, err := parse(name)
	if err != nil {
		return err
	}
	process.Store(&configured{name: point, flow: flow})
	return nil
}
