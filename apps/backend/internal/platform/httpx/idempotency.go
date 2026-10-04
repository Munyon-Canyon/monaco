package httpx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const (
	IdempotencyKeyHeader = "Idempotency-Key"
	IdempotentExtension  = "x-idempotent"
	maxIdempotencyKeyLen = 255
	storeTimeout         = 10 * time.Second
)

type IdempotencyStore interface {
	Begin(ctx context.Context, actorKey, key string, requestHash []byte) (db.Claim, error)
	Complete(ctx context.Context, actorKey, key string, resp db.StoredResponse) error
	Release(ctx context.Context, actorKey, key string) error
}

func Idempotency(store IdempotencyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !mutating(r.Method) || optedOut(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get(IdempotencyKeyHeader)
			if key == "" || len(key) > maxIdempotencyKeyLen {
				Problem(w, r, errs.New(errs.CodeInvalidInput, "httpx.Idempotency",
					slog.String("header", IdempotencyKeyHeader), slog.Int("len", len(key))))
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				invalidRequest(w, r, err)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			hash := requestHash(r, body)
			claimed{
				store: store, w: w, r: r, key: key, hash: hash,
				actor: actorKeyFrom(r.Context(), hash, r.Header.Get("Authorization")),
			}.serve(r.Context(), next)
		})
	}
}

func optedOut(ctx context.Context) bool {
	res, ok := routeFrom(ctx)
	if !ok {
		return false
	}
	declared, isBool := res.route.Operation.Extensions[IdempotentExtension].(bool)
	return isBool && !declared
}

func mutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

func actorKeyFrom(ctx context.Context, requestHash []byte, authorization string) string {
	if a, ok := auth.ActorFrom(ctx); ok {
		return a.Key()
	}
	h := sha256.New()
	_, _ = h.Write(requestHash)
	_, _ = io.WriteString(h, "\n"+authorization)
	return "anonymous:" + hex.EncodeToString(h.Sum(nil))
}

func requestHash(r *http.Request, body []byte) []byte {
	h := sha256.New()
	_, _ = io.WriteString(h, r.Method+"\n"+r.URL.RequestURI()+"\n")
	_, _ = h.Write(idempotencyBody(r, body))
	return h.Sum(nil)
}

func idempotencyBody(r *http.Request, body []byte) []byte {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return body
	}
	var canonical bytes.Buffer
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return canonical.Bytes()
		}
		if err != nil {
			return body
		}
		partBody, err := io.ReadAll(part)
		if err != nil {
			return body
		}
		_ = binary.Write(&canonical, binary.BigEndian, uint64(len(part.FormName())))
		_, _ = io.WriteString(&canonical, part.FormName())
		_ = binary.Write(&canonical, binary.BigEndian, uint64(len(partBody)))
		_, _ = canonical.Write(partBody)
	}
}

type claimed struct {
	store IdempotencyStore
	w     http.ResponseWriter
	r     *http.Request
	key   string
	actor string
	hash  []byte
}

func (c claimed) serve(ctx context.Context, next http.Handler) {
	const op = "httpx.Idempotency"
	claim, err := c.store.Begin(ctx, c.actor, c.key, c.hash)
	if err != nil {
		Problem(c.w, c.r, err)
		return
	}
	attr := slog.String("idempotency_key", c.key)
	switch claim.Outcome {
	case db.ClaimOwned:
		c.execute(ctx, next)
	case db.ClaimInFlight:
		Problem(c.w, c.r, errs.New(errs.CodeIdempotencyInFlight, op, attr))
	case db.ClaimMismatch:
		Problem(c.w, c.r, errs.New(errs.CodeIdempotencyMismatch, op, attr))
	case db.ClaimCompleted:
		observability.Info(ctx, observability.HTTPIdempotencyReplayed,
			slog.String("idempotency_key", c.key), slog.Int("status", claim.Response.Status))
		writeStored(c.w, claim.Response)
	default:
		Problem(c.w, c.r, errs.New(errs.CodeInternal, op, attr, slog.Int("outcome", int(claim.Outcome))))
	}
}

func (c claimed) execute(ctx context.Context, next http.Handler) {
	rec := &capture{header: http.Header{}}
	finished := false
	defer func() {
		if !finished {
			_ = c.release(ctx)
		}
	}()
	next.ServeHTTP(rec, c.r)
	finished = true
	resp := rec.stored()
	if resp.Status >= http.StatusInternalServerError {
		observability.Info(ctx, observability.HTTPIdempotencyReleased,
			slog.String("idempotency_key", c.key), slog.Int("status", resp.Status))
		c.logStoreFailure(ctx, c.release(ctx), resp.Status)
	} else {
		c.logStoreFailure(ctx, c.complete(ctx, resp), resp.Status)
	}
	writeStored(c.w, resp)
}

func (c claimed) release(ctx context.Context) error {
	ctx, cancel := storeContext(ctx)
	defer cancel()
	return c.store.Release(ctx, c.actor, c.key)
}

func (c claimed) complete(ctx context.Context, resp db.StoredResponse) error {
	ctx, cancel := storeContext(ctx)
	defer cancel()
	return c.store.Complete(ctx, c.actor, c.key, resp)
}

func (c claimed) logStoreFailure(ctx context.Context, err error, status int) {
	if err == nil {
		return
	}
	boundary.Error(ctx, observability.HTTPIdempotencyStoreFailed,
		slog.String("idempotency_key", c.key), slog.Int("status", status), slog.Any("err", err))
}

func storeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
}

func writeStored(w http.ResponseWriter, resp db.StoredResponse) {
	maps.Copy(w.Header(), resp.Header)
	w.WriteHeader(resp.Status)
	_, _ = w.Write(resp.Body)
}

type capture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *capture) Header() http.Header { return c.header }

func (c *capture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *capture) Write(b []byte) (int, error) {
	c.WriteHeader(http.StatusOK)
	c.body.Write(b)
	return len(b), nil
}

func (c *capture) stored() db.StoredResponse {
	c.WriteHeader(http.StatusOK)
	return db.StoredResponse{Status: c.status, Header: c.header, Body: c.body.Bytes()}
}
