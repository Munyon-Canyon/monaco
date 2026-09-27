//go:build !faultpoints

package faultpoint

import "context"

const Enabled = false

func Hit(context.Context, Name) {}

func ArmedAfter(ctx context.Context, _ Name, _ int) context.Context {
	return ctx
}

func Configure(name string) error {
	if name == "" {
		return nil
	}
	if err := checkKnown(name); err != nil {
		return err
	}
	return refuse(name, "built without -tags faultpoints")
}
