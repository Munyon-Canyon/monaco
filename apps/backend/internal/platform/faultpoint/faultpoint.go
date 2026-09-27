package faultpoint

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Name string

const (
	AfterCreate  Name = "after-create"
	AfterSign    Name = "after-sign"
	AfterExecute Name = "after-execute"
	BeforeCommit Name = "before-commit"
	AfterPublish Name = "after-publish"
)

func Names() []Name {
	return []Name{AfterCreate, AfterExecute, AfterPublish, AfterSign, BeforeCommit}
}

func Known(name string) bool {
	return slices.Contains(Names(), Name(name))
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
