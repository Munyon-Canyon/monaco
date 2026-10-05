package faultpoint

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Name string

const (
	AfterCreate    Name = "after-create"
	AfterSign      Name = "after-sign"
	AfterBroadcast Name = "after-broadcast"
	AfterExecute   Name = "after-execute"
	BeforeCommit   Name = "before-commit"
	AfterPublish   Name = "after-publish"

	AfterSellRequest Name = "after-sell-request"
	AfterSellConfirm Name = "after-sell-confirm"
)

func Names() []Name {
	return []Name{
		AfterBroadcast, AfterCreate, AfterExecute, AfterPublish, AfterSellConfirm, AfterSellRequest, AfterSign,
		BeforeCommit,
	}
}

func Known(name string) bool {
	return slices.Contains(Names(), Name(name))
}

func Armed(ctx context.Context, name Name) context.Context {
	return ArmedAfter(ctx, name, 0)
}

type flowKey struct{}

func WithFlow(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, flowKey{}, id)
}

func Flow(ctx context.Context) string {
	id, _ := ctx.Value(flowKey{}).(string)
	return id
}

type Crash struct {
	Name Name
}

func (c Crash) Error() string {
	return "faultpoint: crash at " + string(c.Name)
}

func IsCrash(p any) bool {
	_, ok := p.(Crash)
	return ok
}

const configureOp = "faultpoint.Configure"

func refuse(name, reason string) error {
	return errs.New(errs.CodeInvalidInput, configureOp, slog.String("faultpoint", name), slog.String("reason", reason))
}

func checkKnown(name string) error {
	if !Known(name) {
		return refuse(name, "unknown faultpoint")
	}
	return nil
}

func parse(name string) (Name, string, error) {
	point, flow, hasFlow := strings.Cut(name, "@")
	if point == "" || (hasFlow && (flow == "" || strings.Contains(flow, "@"))) {
		return "", "", refuse(name, "invalid faultpoint")
	}
	if err := checkKnown(point); err != nil {
		return "", "", err
	}
	return Name(point), flow, nil
}
