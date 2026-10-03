//go:build !faultpoints

package faultpoint

import "context"

const Enabled = false

func ConfiguredFlow() string { return "" }

func Hit(context.Context, Name) {}

func ArmedAfter(ctx context.Context, _ Name, _ int) context.Context {
	return ctx
}

func Configure(name string) error {
	if name == "" {
		return nil
	}
	if _, _, err := parse(name); err != nil {
		return err
	}
	return refuse(name, "built without -tags faultpoints")
}
