package agents_test

import (
	"errors"
	"log/slog"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/domain"
)

func TestNext(t *testing.T) {
	t.Parallel()
	const none domain.Status = ""
	active, paused, removed := domain.StatusActive, domain.StatusPaused, domain.StatusRemoved
	add, pause, resume, remove := domain.ChangeAdd, domain.ChangePause, domain.ChangeResume, domain.ChangeRemove
	for _, tt := range []struct {
		from   domain.Status
		change domain.Change
		to     domain.Status
		code   errs.Code
	}{
		{none, add, active, ""},
		{removed, add, active, ""},
		{active, pause, paused, ""},
		{paused, resume, active, ""},
		{active, remove, removed, ""},
		{paused, remove, removed, ""},
		{none, pause, "", errs.CodeAgentNotFound},
		{none, resume, "", errs.CodeAgentNotFound},
		{none, remove, "", errs.CodeAgentNotFound},
		{removed, pause, "", errs.CodeAgentNotFound},
		{removed, resume, "", errs.CodeAgentNotFound},
		{removed, remove, "", errs.CodeAgentNotFound},
		{active, add, "", errs.CodeAgentExists},
		{paused, add, "", errs.CodeAgentExists},
		{active, resume, "", errs.CodeAgentWrongStatus},
		{paused, pause, "", errs.CodeAgentWrongStatus},
		{"suspended", add, "", errs.CodeInternal},
		{none, "rename", "", errs.CodeInternal},
		{removed, "rename", "", errs.CodeInternal},
		{active, "rename", "", errs.CodeInternal},
		{paused, "rename", "", errs.CodeInternal},
	} {
		got, err := domain.Next(tt.from, tt.change)
		if tt.code == "" && (err != nil || got != tt.to) {
			t.Errorf("Next(%q, %q) = %q, %v; want %q", tt.from, tt.change, got, err, tt.to)
		}
		if tt.code != "" && (err == nil || errs.CodeOf(err) != tt.code) {
			t.Errorf("Next(%q, %q) = %q, %v; want %s", tt.from, tt.change, got, err, tt.code)
		}
	}
}

func TestNext_errorNamesTheStatusAndTheChange(t *testing.T) {
	t.Parallel()
	_, err := domain.Next(domain.StatusActive, domain.ChangeAdd)
	var e *errs.Error
	want := []slog.Attr{slog.String("from", "active"), slog.String("change", "add")}
	if !errors.As(err, &e) || e.Op != "agents.Next" || !slices.EqualFunc(e.Attrs, want, slog.Attr.Equal) {
		t.Fatalf("Next(active, add) err = %#v, want agents.Next with %v", err, want)
	}
}
