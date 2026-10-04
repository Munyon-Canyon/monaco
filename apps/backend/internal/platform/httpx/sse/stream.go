package sse

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/platformapi"
)

const Heartbeat = 15 * time.Second

type Stream struct {
	hub   *Hub
	clock clock.Clock
}

func NewStream(hub *Hub, c clock.Clock) Stream { return Stream{hub: hub, clock: c} }

type visit func(http.ResponseWriter) error

func (v visit) VisitGetStreamResponse(w http.ResponseWriter) error { return v(w) }

func (s Stream) GetStream(
	ctx context.Context,
	req platformapi.GetStreamRequestObject,
) (platformapi.GetStreamResponseObject, error) {
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return nil, errs.New(errs.CodeUnauthorized, "sse.Stream.GetStream")
	}
	sub, err := s.hub.Register(ctx, actor)
	if err != nil {
		return nil, err
	}
	resync := req.Params.LastEventID != nil
	return visit(func(w http.ResponseWriter) error {
		defer sub.Close()
		s.serve(ctx, w, sub, resync)
		return nil
	}), nil
}

func (s Stream) serve(ctx context.Context, w http.ResponseWriter, sub *Subscription, resync bool) {
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	out := &frames{w: w, rc: rc}
	out.flush()
	if resync {
		out.write("event: resync\ndata: {}\n\n")
	}
	ticker := s.clock.NewTicker(Heartbeat)
	defer ticker.Stop()
	var id uint64
	for out.err == nil {
		select {
		case <-ctx.Done():
			return
		case hint, ok := <-sub.Hints():
			if !ok {
				return
			}
			id++
			out.write("id: " + strconv.FormatUint(id, 10) + "\nevent: hint\ndata: {\"key\":\"" + string(hint.Key) +
				"\",\"what\":\"" + hint.What + "\"}\n\n")
		case <-ticker.C():
			out.write(": heartbeat\n\n")
		}
	}
}

type frames struct {
	w   http.ResponseWriter
	rc  *http.ResponseController
	err error
}

func (f *frames) write(frame string) {
	if _, err := f.w.Write([]byte(frame)); err != nil {
		f.err = err
		return
	}
	f.flush()
}

func (f *frames) flush() { f.err = f.rc.Flush() }
