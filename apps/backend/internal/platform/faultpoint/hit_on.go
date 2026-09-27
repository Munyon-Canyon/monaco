//go:build faultpoints

package faultpoint

import (
	"context"
	"sync/atomic"
)

const Enabled = true

type armedKey struct{}

var process atomic.Pointer[Name]

func Hit(ctx context.Context, name Name) {
	if p := process.Load(); p != nil && *p == name {
		panic(Crash{Name: name})
	}
	if armed, ok := ctx.Value(armedKey{}).(Name); ok && armed == name {
		panic(Crash{Name: name})
	}
}

func Armed(ctx context.Context, name Name) context.Context {
	return context.WithValue(ctx, armedKey{}, name)
}

func Configure(name string) error {
	if name == "" {
		process.Store(nil)
		return nil
	}
	if err := checkKnown(name); err != nil {
		return err
	}
	armed := Name(name)
	process.Store(&armed)
	return nil
}
