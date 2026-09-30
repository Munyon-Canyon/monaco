package apns

import (
	"context"
	"net/http"
	"time"

	"github.com/sideshow/apns2"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Environment string

const (
	Sandbox    Environment = "sandbox"
	Production Environment = "production"
)

type Push struct {
	UserID      ids.UserID
	Token       string
	Environment Environment
	CollapseID  string
	Title       string
	Body        string
	Data        map[string]string
}

type Result struct {
	Status     int
	Reason     string
	APNsID     string
	RetryAfter time.Duration
}

type Outcome uint8

const (
	Delivered Outcome = iota + 1
	TokenDead
	Retry
	AuthFailed
	Rejected
)

type Sender interface {
	Send(ctx context.Context, p Push) (Result, error)
}

func Classify(r Result) Outcome {
	switch {
	case r.Status == http.StatusOK:
		return Delivered
	case r.Status == http.StatusGone,
		r.Status == http.StatusBadRequest && r.Reason == apns2.ReasonBadDeviceToken:
		return TokenDead
	case r.Status == http.StatusTooManyRequests, r.Status/100 == 5:
		return Retry
	case r.Status == http.StatusForbidden:
		return AuthFailed
	}
	return Rejected
}
