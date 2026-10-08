package adapters

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const globalTargetID = "global"

func (h HTTP) PauseAdminCabal(
	ctx context.Context, req api.PauseAdminCabalRequestObject,
) (api.PauseAdminCabalResponseObject, error) {
	state, err := h.pause(ctx, &req.Id, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return api.PauseAdminCabal200JSONResponse(state), nil
}

func (h HTTP) ResumeAdminCabal(
	ctx context.Context, req api.ResumeAdminCabalRequestObject,
) (api.ResumeAdminCabalResponseObject, error) {
	state, err := h.resume(ctx, &req.Id, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return api.ResumeAdminCabal200JSONResponse(state), nil
}

func (h HTTP) PauseAdminAll(
	ctx context.Context, req api.PauseAdminAllRequestObject,
) (api.PauseAdminAllResponseObject, error) {
	state, err := h.pause(ctx, nil, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return api.PauseAdminAll200JSONResponse(state), nil
}

func (h HTTP) ResumeAdminAll(
	ctx context.Context, req api.ResumeAdminAllRequestObject,
) (api.ResumeAdminAllResponseObject, error) {
	state, err := h.resume(ctx, nil, req.Body.Reason)
	if err != nil {
		return nil, err
	}
	return api.ResumeAdminAll200JSONResponse(state), nil
}

func (h HTTP) pause(ctx context.Context, cabal *uuid.UUID, text string) (api.PauseState, error) {
	scope, err := h.adminScope(ctx, cabal, text, events.AdminActionOpsPause, events.AdminActionGlobalPause)
	if err != nil {
		return api.PauseState{}, err
	}
	_, err = h.Pause.Handle(ctx, app.PauseCabal{
		CabalID: scope.cabal, Reason: domain.PauseReasonOps, Note: scope.reason, Actor: &scope.admin,
		AdminAction: &scope.action,
	})
	if err != nil {
		return api.PauseState{}, err
	}
	return h.state(ctx, scope.cabal)
}

func (h HTTP) resume(ctx context.Context, cabal *uuid.UUID, text string) (api.PauseState, error) {
	scope, err := h.adminScope(ctx, cabal, text, events.AdminActionOpsResume, events.AdminActionGlobalResume)
	if err != nil {
		return api.PauseState{}, err
	}
	err = h.Resume.Handle(ctx, app.ResumeCabal{CabalID: scope.cabal, Actor: &scope.admin, AdminAction: &scope.action})
	if err != nil {
		return api.PauseState{}, err
	}
	return h.state(ctx, scope.cabal)
}

type adminScope struct {
	admin  ids.UserID
	cabal  *ids.CabalID
	reason string
	action events.AdminAction
}

func (h HTTP) adminScope(
	ctx context.Context, cabal *uuid.UUID, text string, perCabal, global events.AdminActionKind,
) (adminScope, error) {
	admin, err := adminCaller(ctx)
	if err != nil {
		return adminScope{}, err
	}
	reason, err := events.NewReason(text)
	if err != nil {
		return adminScope{}, err
	}
	scope := adminScope{admin: admin, reason: reason.String()}
	kind, target, targetID := global, events.AdminTargetGlobal, globalTargetID
	if cabal != nil {
		id := ids.CabalIDFrom(*cabal)
		if err := h.Cabals.Exists(ctx, id); err != nil {
			return adminScope{}, err
		}
		scope.cabal = &id
		kind, target, targetID = perCabal, events.AdminTargetCabal, id.String()
	}
	scope.action, err = events.NewAdminAction(h.IDs.NewV7(), admin, kind, target, targetID, reason, nil, nil)
	return scope, err
}

func (h HTTP) state(ctx context.Context, cabal *ids.CabalID) (api.PauseState, error) {
	scope := ids.CabalID{}
	if cabal != nil {
		scope = *cabal
	}
	pause, err := NewPauses(h.Reads).IsPaused(ctx, scope)
	if err != nil {
		return api.PauseState{}, err
	}
	return pauseState(pause), nil
}

func pauseState(p port.Pause) api.PauseState {
	state := api.PauseState{Paused: p.Paused, Reasons: make([]api.PauseReason, len(p.Reasons))}
	for i, r := range p.Reasons {
		state.Reasons[i] = api.PauseReason(r)
	}
	if p.Paused {
		since := p.Since.UTC()
		state.Since = &since
	}
	return state
}

func adminCaller(ctx context.Context) (ids.UserID, error) {
	const op = "funding.adminCaller"
	actor, ok := auth.ActorFrom(ctx)
	if !ok || actor.Kind != auth.ActorAdmin {
		return ids.UserID{}, errs.New(errs.CodeAdminForbidden, op)
	}
	admin, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeAdminForbidden, op)
	}
	return admin, nil
}
