package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusPaused  Status = "paused"
	StatusRemoved Status = "removed"
)

type Change string

const (
	ChangeAdd    Change = "add"
	ChangePause  Change = "pause"
	ChangeResume Change = "resume"
	ChangeRemove Change = "remove"
)

func Next(from Status, change Change) (Status, error) {
	switch from {
	case StatusActive:
		return nextFromActive(from, change)
	case StatusPaused:
		return nextFromPaused(from, change)
	case StatusRemoved, "":
		return nextFromNone(from, change)
	}
	return from, refuse(errs.CodeInternal, from, change)
}

func nextFromNone(from Status, change Change) (Status, error) {
	switch change {
	case ChangeAdd:
		return StatusActive, nil
	case ChangePause, ChangeResume, ChangeRemove:
		return from, refuse(errs.CodeAgentNotFound, from, change)
	}
	return from, refuse(errs.CodeInternal, from, change)
}

func nextFromActive(from Status, change Change) (Status, error) {
	switch change {
	case ChangePause:
		return StatusPaused, nil
	case ChangeRemove:
		return StatusRemoved, nil
	case ChangeAdd:
		return from, refuse(errs.CodeAgentExists, from, change)
	case ChangeResume:
		return from, refuse(errs.CodeAgentWrongStatus, from, change)
	}
	return from, refuse(errs.CodeInternal, from, change)
}

func nextFromPaused(from Status, change Change) (Status, error) {
	switch change {
	case ChangeResume:
		return StatusActive, nil
	case ChangeRemove:
		return StatusRemoved, nil
	case ChangeAdd:
		return from, refuse(errs.CodeAgentExists, from, change)
	case ChangePause:
		return from, refuse(errs.CodeAgentWrongStatus, from, change)
	}
	return from, refuse(errs.CodeInternal, from, change)
}

func refuse(code errs.Code, from Status, change Change) error {
	return errs.New(code, "agents.Next", slog.String("from", string(from)), slog.String("change", string(change)))
}
